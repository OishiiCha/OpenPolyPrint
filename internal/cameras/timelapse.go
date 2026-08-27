package cameras

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	framesDir string
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
	framesDir := filepath.Join("recordings/timelapse", base+"_frames")

	// Create frames directory for individual JPEG frame extraction
	if err := os.MkdirAll(framesDir, 0o755); err != nil {
		return "", fmt.Errorf("create frames dir: %w", err)
	}

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
		framesDir: framesDir,
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
			frameNum := atomic.AddInt64(&r.frames, 1)
			// Also save individual frame as JPEG with timestamp metadata
			r.saveFrame(frame, frameNum)
		}
	}
}

// saveFrame writes an individual frame as a JPEG file with a timestamp-based
// filename. The frame data from the MJPEG streamer is already JPEG-encoded.
// A companion .json file is written with metadata (timestamp, frame number,
// elapsed seconds) for AI analysis.
func (r *tlRecording) saveFrame(frame []byte, frameNum int64) {
	elapsed := time.Since(r.startTime).Seconds()
	timestamp := time.Now().UnixMilli()

	// Save frame as JPEG
	framePath := filepath.Join(r.framesDir, fmt.Sprintf("frame_%06d.jpg", frameNum))
	if err := os.WriteFile(framePath, frame, 0o644); err != nil {
		log.Printf("[timelapse] failed to save frame %d: %v", frameNum, err)
		return
	}

	// Save metadata as JSON
	metaPath := filepath.Join(r.framesDir, fmt.Sprintf("frame_%06d.json", frameNum))
	meta := fmt.Sprintf(`{"frame":%d,"timestamp":%d,"elapsedSeconds":%.3f,"cameraId":"%s"}`,
		frameNum, timestamp, elapsed, sanitizeFilename(r.cameraID))
	_ = os.WriteFile(metaPath, []byte(meta), 0o644)
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

// ListFrameDirs returns the frame directories in recordings/timelapse.
func ListFrameDirs() ([]string, error) {
	entries, err := os.ReadDir("recordings/timelapse")
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), "_frames") {
			dirs = append(dirs, e.Name())
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i] > dirs[j] })
	return dirs, nil
}

// ListFrames returns the JPEG frames in a frame directory.
func ListFrames(dir string) ([]string, error) {
	path := filepath.Join("recordings/timelapse", dir)
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(strings.ToLower(name), ".jpg") {
			files = append(files, name)
		}
	}
	sort.Strings(files)
	return files, nil
}

// FrameMeta is the metadata for a single timelapse frame.
type FrameMeta struct {
	Frame          int     `json:"frame"`
	Timestamp      int64   `json:"timestamp"`      // unix millis
	ElapsedSeconds float64 `json:"elapsedSeconds"` // seconds since recording start
	CameraID       string  `json:"cameraId"`
}

// ListFrameMeta returns metadata for all frames in a frame directory.
// It reads the companion .json files written alongside each .jpg frame.
func ListFrameMeta(dir string) ([]FrameMeta, error) {
	path := filepath.Join("recordings/timelapse", dir)
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var metas []FrameMeta
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(path, name))
		if err != nil {
			continue
		}
		var meta FrameMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		metas = append(metas, meta)
	}
	sort.Slice(metas, func(i, j int) bool { return metas[i].Frame < metas[j].Frame })
	return metas, nil
}
