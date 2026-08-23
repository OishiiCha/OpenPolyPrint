package cameras

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// RecordManager tracks active camera recordings.
type RecordManager struct {
	mu   sync.Mutex
	recs map[string]*recording
}

// NewRecordManager creates a new RecordManager.
func NewRecordManager() *RecordManager {
	return &RecordManager{recs: make(map[string]*recording)}
}

type recording struct {
	cameraID  string
	rawPath   string
	outPath   string
	rawFile   *os.File
	sub       chan []byte
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan error
	startTime time.Time
	frames    int64
}

// RecordStatus describes an active video recording.
type RecordStatus struct {
	Active         bool      `json:"active"`
	StartTime      time.Time `json:"startTime"`
	Frames         int64     `json:"frames"`
	ElapsedSeconds float64   `json:"elapsedSeconds"`
}

// Start begins recording the given camera's MJPEG stream to ./recordings/videos.
// Frames are written to a .mjpeg file and converted to the supplied MKV
// filename when Stop is called.
func (rm *RecordManager) Start(cameraID string, streamer *UsbCameraStreamer, filename string) (string, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, ok := rm.recs[cameraID]; ok {
		return "", fmt.Errorf("already recording camera %s", cameraID)
	}
	if streamer == nil {
		return "", fmt.Errorf("camera stream not available")
	}
	if !streamer.IsRunning() {
		return "", fmt.Errorf("camera stream is not running")
	}

	if err := os.MkdirAll("recordings/videos", 0o755); err != nil {
		return "", fmt.Errorf("create videos dir: %w", err)
	}

	if filename == "" {
		filename = fmt.Sprintf("%s-%s.mkv", sanitizeFilename(cameraID), time.Now().Format("20060102-1504"))
	}
	filename = sanitizeFilename(filename)

	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	rawPath := filepath.Join("recordings/videos", base+".mjpeg")
	outPath := filepath.Join("recordings/videos", filename)

	f, err := os.Create(rawPath)
	if err != nil {
		return "", fmt.Errorf("create recording file: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	sub := streamer.Subscribe()
	rec := &recording{
		cameraID:  cameraID,
		rawPath:   rawPath,
		outPath:   outPath,
		rawFile:   f,
		sub:       sub,
		ctx:       ctx,
		cancel:    cancel,
		done:      make(chan error, 1),
		startTime: time.Now(),
	}
	rm.recs[cameraID] = rec

	go rec.run(streamer)
	log.Printf("[record] started recording %s to %s", cameraID, outPath)
	return outPath, nil
}

func (r *recording) run(streamer *UsbCameraStreamer) {
	defer close(r.done)

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
			if _, err := r.rawFile.Write(frame); err != nil {
				finish(true)
				return
			}
			atomic.AddInt64(&r.frames, 1)
		}
	}
}

// convert packages the raw MJPEG frames into a browser-playable MKV file with ffmpeg.
func (r *recording) convert() error {
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

// Stop stops the active recording for the given camera.
func (rm *RecordManager) Stop(cameraID string) (string, error) {
	rm.mu.Lock()
	rec, ok := rm.recs[cameraID]
	if !ok {
		rm.mu.Unlock()
		return "", fmt.Errorf("no active recording for %s", cameraID)
	}
	delete(rm.recs, cameraID)
	rm.mu.Unlock()

	rec.cancel()
	select {
	case err := <-rec.done:
		if err != nil {
			log.Printf("[record] %s conversion error: %v", cameraID, err)
			if rec.outPath == "" || rec.rawPath == "" {
				return "", err
			}
			// Return the raw file if conversion failed so nothing is lost.
			return rec.rawPath, err
		}
		log.Printf("[record] stopped recording %s -> %s", cameraID, rec.outPath)
		return rec.outPath, nil
	case <-time.After(60 * time.Second):
		return rec.rawPath, fmt.Errorf("recording stop timed out")
	}
}

// IsRecording reports whether a recording is currently active for the camera.
func (rm *RecordManager) IsRecording(cameraID string) bool {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	_, ok := rm.recs[cameraID]
	return ok
}

// Status returns the status of the active recording for the camera.
func (rm *RecordManager) Status(cameraID string) RecordStatus {
	rm.mu.Lock()
	rec, ok := rm.recs[cameraID]
	rm.mu.Unlock()

	if !ok {
		return RecordStatus{}
	}
	return RecordStatus{
		Active:         true,
		StartTime:      rec.startTime,
		Frames:         atomic.LoadInt64(&rec.frames),
		ElapsedSeconds: time.Since(rec.startTime).Seconds(),
	}
}

func listRecordings(dir string) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(strings.ToLower(name), ".mp4") ||
			strings.HasSuffix(strings.ToLower(name), ".avi") ||
			strings.HasSuffix(strings.ToLower(name), ".mkv") {
			files = append(files, name)
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i] > files[j] })
	return files, nil
}

// ListVideos returns the video recordings in recordings/videos.
func ListVideos() ([]string, error) {
	return listRecordings("recordings/videos")
}

// ListTimelapses returns the timelapse recordings in recordings/timelapse.
func ListTimelapses() ([]string, error) {
	return listRecordings("recordings/timelapse")
}

// ListRecordings returns all recordings (legacy, returns videos).
func ListRecordings() ([]string, error) {
	return ListVideos()
}

func ServeThumb(folder, filename string) ([]byte, string, error) {
	path := filepath.Join("recordings", folder, filename)
	if _, err := os.Stat(path); err != nil {
		return nil, "", fmt.Errorf("file not found: %w", err)
	}

	duration := 0.0
	out, err := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	).Output()
	if err == nil {
		if d, parseErr := strconv.ParseFloat(strings.TrimSpace(string(out)), 64); parseErr == nil {
			duration = d
		}
	}

	seek := duration / 2
	cmd := exec.Command("ffmpeg",
		"-ss", fmt.Sprintf("%.3f", seek),
		"-i", path,
		"-vframes", "1",
		"-f", "image2",
		"-vcodec", "mjpeg",
		"-q:v", "5",
		"-",
	)
	cmd.Stderr = io.Discard
	thumb, err := cmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("generate thumbnail: %w", err)
	}
	return thumb, "image/jpeg", nil
}

// ConvertToTimelapse speeds up the source recording and writes it to recordings/timelapse.
func ConvertToTimelapse(folder, filename string) (string, error) {
	src := filepath.Join("recordings", folder, filename)
	if _, err := os.Stat(src); err != nil {
		return "", fmt.Errorf("file not found: %w", err)
	}

	if err := os.MkdirAll("recordings/timelapse", 0o755); err != nil {
		return "", fmt.Errorf("create timelapse dir: %w", err)
	}

	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	out := filepath.Join("recordings/timelapse", base+"_timelapse.mkv")

	cmd := exec.Command("ffmpeg",
		"-i", src,
		"-vf", "select='not(mod(n,10))',setpts=N/(30*TB)",
		"-c:v", "libx264",
		"-pix_fmt", "yuv420p",
		"-r", "30",
		"-an",
		"-f", "matroska",
		"-y", out,
	)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("convert to timelapse: %w", err)
	}
	return out, nil
}

func sanitizeFilename(s string) string {
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	return s
}
