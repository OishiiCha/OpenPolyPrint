package cameras

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// TimelapseManager tracks active camera timelapse recordings.
type TimelapseManager struct {
	mu   sync.Mutex
	recs map[string]*tlRecording
}

// NewTimelapseManager creates a new TimelapseManager.
func NewTimelapseManager() *TimelapseManager {
	return &TimelapseManager{recs: make(map[string]*tlRecording)}
}

type tlRecording struct {
	cameraID  string
	rawPath   string
	outPath   string
	rawFile   *os.File
	sub       chan []byte
	interval  time.Duration
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan error
	startTime time.Time
	frames    int64

	mu        sync.RWMutex
	lastFrame []byte
}

// TimelapseStatus describes an active timelapse recording.
type TimelapseStatus struct {
	Active          bool      `json:"active"`
	StartTime       time.Time `json:"startTime"`
	Frames          int64     `json:"frames"`
	IntervalSeconds float64   `json:"intervalSeconds"`
	NextCapture     time.Time `json:"nextCapture"`
	ElapsedSeconds  float64   `json:"elapsedSeconds"`
}

// Start begins a timelapse recording. It captures one frame every intervalSeconds
// and writes the resulting MKV to ./recordings/timelapse.
func (rm *TimelapseManager) Start(cameraID string, streamer *UsbCameraStreamer, filename string, intervalSeconds float64) (string, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, ok := rm.recs[cameraID]; ok {
		return "", fmt.Errorf("already recording timelapse for camera %s", cameraID)
	}
	if streamer == nil {
		return "", fmt.Errorf("camera stream not available")
	}
	if !streamer.IsRunning() {
		return "", fmt.Errorf("camera stream is not running")
	}
	if intervalSeconds <= 0 {
		return "", fmt.Errorf("interval must be > 0")
	}

	if err := os.MkdirAll("recordings/timelapse", 0o755); err != nil {
		return "", fmt.Errorf("create timelapse dir: %w", err)
	}

	if filename == "" {
		filename = fmt.Sprintf("%s-%s.mkv", sanitizeFilename(cameraID), time.Now().Format("20060102-1504"))
	}
	filename = sanitizeFilename(filename)

	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	rawPath := filepath.Join("recordings/timelapse", base+".mjpeg")
	outPath := filepath.Join("recordings/timelapse", filename)

	f, err := os.Create(rawPath)
	if err != nil {
		return "", fmt.Errorf("create timelapse file: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	sub := streamer.Subscribe()
	rec := &tlRecording{
		cameraID:  cameraID,
		rawPath:   rawPath,
		outPath:   outPath,
		rawFile:   f,
		sub:       sub,
		interval:  time.Duration(intervalSeconds * float64(time.Second)),
		ctx:       ctx,
		cancel:    cancel,
		done:      make(chan error, 1),
		startTime: time.Now(),
	}
	rm.recs[cameraID] = rec

	go rec.run(streamer)
	log.Printf("[timelapse] started %s to %s (interval %.2fs)", cameraID, outPath, intervalSeconds)
	return outPath, nil
}

func (r *tlRecording) run(streamer *UsbCameraStreamer) {
	defer close(r.done)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	finish := func(convert bool) {
		streamer.Unsubscribe(r.sub)
		_ = r.rawFile.Close()
		if !convert {
			r.done <- nil
			return
		}
		r.done <- r.convert()
	}

	for {
		select {
		case <-r.ctx.Done():
			finish(true)
			return
		case frame, ok := <-r.sub:
			if !ok {
				finish(true)
				return
			}
			r.mu.Lock()
			r.lastFrame = frame
			r.mu.Unlock()
		case <-ticker.C:
			r.mu.RLock()
			frame := r.lastFrame
			r.mu.RUnlock()
			if frame == nil {
				continue
			}
			if _, err := r.rawFile.Write(frame); err != nil {
				finish(true)
				return
			}
			atomic.AddInt64(&r.frames, 1)
		}
	}
}

// convert packages the raw MJPEG frames into a browser-playable MKV file with ffmpeg.
func (r *tlRecording) convert() error {
	cmd := exec.Command("ffmpeg",
		"-f", "mjpeg", "-r", "30", "-i", r.rawPath,
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-r", "30",
		"-f", "matroska", "-y", r.outPath,
	)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg: %w", err)
	}
	_ = os.Remove(r.rawPath)
	return nil
}

// Stop stops the active timelapse recording for the given camera.
func (rm *TimelapseManager) Stop(cameraID string) (string, error) {
	rm.mu.Lock()
	rec, ok := rm.recs[cameraID]
	if !ok {
		rm.mu.Unlock()
		return "", fmt.Errorf("no active timelapse for %s", cameraID)
	}
	delete(rm.recs, cameraID)
	rm.mu.Unlock()

	rec.cancel()
	select {
	case err := <-rec.done:
		if err != nil {
			log.Printf("[timelapse] %s conversion error: %v", cameraID, err)
			if rec.outPath == "" || rec.rawPath == "" {
				return "", err
			}
			return rec.rawPath, err
		}
		log.Printf("[timelapse] stopped %s -> %s", cameraID, rec.outPath)
		return rec.outPath, nil
	case <-time.After(60 * time.Second):
		return rec.rawPath, fmt.Errorf("timelapse stop timed out")
	}
}

// IsRecording reports whether a timelapse is currently active for the camera.
func (rm *TimelapseManager) IsRecording(cameraID string) bool {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	_, ok := rm.recs[cameraID]
	return ok
}

// Status returns the status of the active timelapse for the camera.
func (rm *TimelapseManager) Status(cameraID string) TimelapseStatus {
	rm.mu.Lock()
	rec, ok := rm.recs[cameraID]
	rm.mu.Unlock()

	if !ok {
		return TimelapseStatus{}
	}
	frames := atomic.LoadInt64(&rec.frames)
	next := rec.startTime.Add(time.Duration(frames+1) * rec.interval)
	return TimelapseStatus{
		Active:          true,
		StartTime:       rec.startTime,
		Frames:          frames,
		IntervalSeconds: rec.interval.Seconds(),
		NextCapture:     next,
		ElapsedSeconds:  time.Since(rec.startTime).Seconds(),
	}
}
