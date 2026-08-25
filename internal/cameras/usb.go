package cameras

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// isNumeric reports whether s contains only digits.
func isNumeric(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// Debugf logs a scoped diagnostic message.
func Debugf(scope, format string, args ...any) {
	log.Printf("[%s] "+format, append([]any{scope}, args...)...)
}

// UsbCameraStreamer manages a persistent ffmpeg process that reads from a USB camera
// and broadcasts JPEG frames to multiple subscribers. This allows simultaneous viewing
// (via WebSocket) and recording without the browser owning the device.
type UsbCameraStreamer struct {
	cameraID   string
	cameraName string
	config     CameraConfig

	mu          sync.Mutex
	cmd         *exec.Cmd
	cancel      context.CancelFunc
	subscribers []chan []byte
	running     bool
	restartCh   chan struct{}
	useAltArgs  bool // set true when fallback args work (e.g. OBS Virtual Camera)
	useMinimal  bool // set true when minimal args (no resolution) work

	// health tracking, guarded by mu
	frames      int64
	lastFrameAt time.Time
	fpsEst      float64
	lastErr     string
	lastFrame   []byte // most recent frame, for instant subscriber pickup
}

// NewUsbCameraStreamer creates a new streamer for the given USB camera config.
func NewUsbCameraStreamer(cam *CameraConfig) *UsbCameraStreamer {
	return &UsbCameraStreamer{
		cameraID:   cam.ID,
		cameraName: cam.Name,
		config:     *cam,
		restartCh:  make(chan struct{}, 1),
	}
}

// Start begins the ffmpeg process and frame broadcasting.
func (u *UsbCameraStreamer) Start() {
	u.mu.Lock()
	if u.running {
		u.mu.Unlock()
		return
	}
	u.running = true
	u.mu.Unlock()

	go u.runLoop()
}

// Stop terminates the ffmpeg process and closes all subscribers.
func (u *UsbCameraStreamer) Stop() {
	u.mu.Lock()
	u.running = false
	if u.cancel != nil {
		u.cancel()
	}
	if u.cmd != nil && u.cmd.Process != nil {
		_ = u.cmd.Process.Kill()
	}
	// Close all subscribers
	for _, sub := range u.subscribers {
		close(sub)
	}
	u.subscribers = nil
	u.mu.Unlock()
}

// Subscribe returns a channel that receives JPEG frames.
// The caller must call Unsubscribe when done to avoid resource leaks.
// If a frame has already been received, it is sent immediately so the
// subscriber sees an image without waiting for the next ffmpeg frame.
func (u *UsbCameraStreamer) Subscribe() chan []byte {
	sub := make(chan []byte, 32)
	u.mu.Lock()
	u.subscribers = append(u.subscribers, sub)
	last := u.lastFrame
	u.mu.Unlock()
	// Send the last frame immediately if we have one, so the subscriber
	// gets an instant first image instead of waiting for the next frame.
	if last != nil {
		select {
		case sub <- last:
		default:
		}
	}
	return sub
}

// Unsubscribe removes a subscriber channel and closes it.
func (u *UsbCameraStreamer) Unsubscribe(sub chan []byte) {
	u.mu.Lock()
	for i, s := range u.subscribers {
		if s == sub {
			u.subscribers = append(u.subscribers[:i], u.subscribers[i+1:]...)
			close(sub)
			break
		}
	}
	u.mu.Unlock()
}

// IsRunning returns whether the streamer is currently active.
func (u *UsbCameraStreamer) IsRunning() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.running
}

// runLoop manages the ffmpeg lifecycle with automatic restart on failure.
func (u *UsbCameraStreamer) runLoop() {
	for {
		u.mu.Lock()
		if !u.running {
			u.mu.Unlock()
			return
		}
		u.mu.Unlock()

		err := u.runFfmpeg()
		if err != nil {
			u.setLastErr(err)
			u.dbgf("capture process exited: %v", err)
		}

		u.mu.Lock()
		stillRunning := u.running
		u.mu.Unlock()

		if !stillRunning {
			return
		}

		// Wait before restarting, or exit if stopped
		select {
		case <-time.After(3 * time.Second):
			u.dbgf("restarting capture process")
		case <-u.restartCh:
			// immediate restart requested
		}
	}
}

// runFfmpeg starts a single ffmpeg process, reads JPEG frames from stdout,
// and broadcasts them to all subscribers. Tries fallback args for virtual cameras.
func (u *UsbCameraStreamer) runFfmpeg() error {
	// Raspberry Pi camera uses rpicam-vid/libcamera-vid directly (no ffmpeg fallback needed)
	if u.config.Type == "rpicam" {
		return u.runRpiCam()
	}

	// If we already know which mode works, use it directly
	if u.useMinimal {
		return u.runFfmpegWithArgs(u.buildMinimalStreamArgs(), false)
	}
	if u.useAltArgs {
		return u.runFfmpegWithArgs(u.buildAltStreamArgs(), true)
	}

	// Level 1: Primary args (MJPEG input, 1280x720 — works for most real USB cameras)
	err := u.runFfmpegWithArgs(u.buildStreamArgs(), false)
	if err == nil {
		return nil
	}
	u.mu.Lock()
	stillRunning := u.running
	u.mu.Unlock()
	if !stillRunning {
		return err
	}

	// Level 2: Alt args (no MJPEG input, 640x480 — works for some virtual cameras)
	u.dbgf("primary args failed (%v), trying fallback (640x480)", err)
	err = u.runFfmpegWithArgs(u.buildAltStreamArgs(), true)
	if err == nil {
		u.useAltArgs = true
		u.dbgf("alt args working, locking in alt mode")
		return nil
	}
	u.mu.Lock()
	stillRunning = u.running
	u.mu.Unlock()
	if !stillRunning {
		return err
	}

	// Level 3: Minimal args (no resolution/format — lets ffmpeg auto-negotiate)
	// Works for OBS Virtual Camera and other virtual cameras that only support specific resolutions
	u.dbgf("alt args failed (%v), trying minimal (auto-negotiate)", err)
	err = u.runFfmpegWithArgs(u.buildMinimalStreamArgs(), false)
	if err == nil {
		u.useMinimal = true
		u.dbgf("minimal args working, locking in minimal mode")
	}
	return err
}

func (u *UsbCameraStreamer) runFfmpegWithArgs(args []string, isAlt bool) error {
	return u.runCommand("ffmpeg", args, isAlt)
}

// runRpiCam starts rpicam-vid to stream MJPEG frames from the Raspberry Pi
// camera module via the MIPI CSI interface. Falls back to ffmpeg + v4l2 if
// rpicam-vid is not installed.
func (u *UsbCameraStreamer) runRpiCam() error {
	args := []string{
		"-t", "0",
		"--codec", "mjpeg",
		"--width", "1280",
		"--height", "720",
		"--framerate", "30",
	}
	// Explicit camera index (from deviceId) or sensor name. The tools
	// only accept a numeric camera index for --camera, so resolve a sensor
	// name via --list-cameras when only a sensor is given.
	if id := strings.TrimSpace(u.config.DeviceID); id != "" && isNumeric(id) {
		args = append(args, "--camera", id)
	} else if sensor := u.config.Sensor; sensor != "" && sensor != "auto" {
		if idx := u.resolveRpiCamIndex(sensor); idx != "" {
			args = append(args, "--camera", idx)
		}
	}
	// Sensor flips: the sensor can only mirror, so 90°/270° orientations are
	// handled browser-side (canvas draw rotation) instead of here.
	switch u.config.Flip {
	case "horizontal":
		args = append(args, "--hflip")
	case "vertical":
		args = append(args, "--vflip")
	case "both":
		args = append(args, "--vflip", "--hflip")
	}
	if b := u.config.Brightness; b != 0 {
		args = append(args, "--brightness", fmt.Sprintf("%.2f", b))
	}
	args = append(args, "-o", "-")

	// Try rpicam-vid (the only Pi camera tool shipped in the Pi image).
	err := u.runCommand("rpicam-vid", args, false)
	if err == nil {
		return nil
	}
	u.mu.Lock()
	stillRunning := u.running
	u.mu.Unlock()
	if !stillRunning {
		return err
	}

	// Final fallback: ffmpeg + v4l2 (works if the Pi camera exposes a /dev/videoN device)
	u.dbgf("rpicam-vid failed (%v), trying ffmpeg v4l2 fallback", err)
	v4l2Args := []string{
		"-f", "v4l2",
		"-framerate", "30",
		"-video_size", "1280x720",
		"-input_format", "mjpeg",
		"-i", "/dev/video0",
		"-c:v", "mjpeg",
		"-q:v", "5",
		"-f", "mjpeg",
		"-",
	}
	return u.runCommand("ffmpeg", v4l2Args, false)
}

// resolveRpiCamIndex maps a sensor name (e.g. "imx219") to the numeric camera
// index that rpicam-vid/libcamera-vid accept for --camera, using
// --list-cameras output. Returns "" when the index cannot be determined, in
// which case --camera is omitted and the default camera is used.
func (u *UsbCameraStreamer) resolveRpiCamIndex(sensor string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var out []byte
	var err error
	out, err = exec.CommandContext(ctx, "rpicam-hello", "--list-cameras").CombinedOutput()
	if err != nil {
		u.dbgf("could not list cameras to resolve sensor %q (%v); omitting --camera", sensor, err)
		return ""
	}

	for _, line := range strings.Split(string(out), "\n") {
		// Lines look like: "0 : imx219 [3280x2464 10-bit RGGB] (/base/soc/...)"
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 3 && fields[1] == ":" &&
			(fields[2] == sensor || strings.HasPrefix(fields[2], sensor)) {
			return fields[0]
		}
	}
	u.dbgf("sensor %q not found in --list-cameras output; omitting --camera", sensor)
	return ""
}

// runCommand starts a process with the given command and args, reads JPEG frames
// from stdout, and broadcasts them to all subscribers.
func (u *UsbCameraStreamer) runCommand(name string, args []string, isAlt bool) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	u.mu.Lock()
	u.cancel = cancel
	u.mu.Unlock()

	// Use custom ffmpeg path only for ffmpeg commands
	cmdName := name

	cmd := exec.CommandContext(ctx, cmdName, args...)
	u.mu.Lock()
	u.cmd = cmd
	u.running = true
	u.mu.Unlock()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", cmdName, err)
	}

	u.dbgf("%s started (pid=%d, alt=%v) args: %v", cmdName, cmd.Process.Pid, isAlt, args)

	reader := bufio.NewReaderSize(stdout, 256*1024)

	// Read JPEG frames from ffmpeg stdout
	// JPEG frames start with 0xFF 0xD8 and end with 0xFF 0xD9
	frameCount := 0
	for {
		u.mu.Lock()
		stillRunning := u.running
		u.mu.Unlock()
		if !stillRunning {
			_ = cmd.Process.Kill()
			return nil
		}

		frame, err := readJpegFrame(reader)
		if err != nil {
			// Check if we were cancelled
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			serr := stderr.String()
			if len(serr) > 2000 {
				serr = serr[:2000]
			}
			return fmt.Errorf("read frame: %w (stderr: %s)", err, serr)
		}

		frameCount++
		if frameCount == 1 {
			u.dbgf("first JPEG frame received from %s (%d bytes)", cmdName, len(frame))
		} else if frameCount%900 == 0 {
			u.dbgf("streaming OK: %d frames received", frameCount)
		}
		now := time.Now()
		u.mu.Lock()
		u.frames++
		if !u.lastFrameAt.IsZero() {
			if dt := now.Sub(u.lastFrameAt).Seconds(); dt > 0 {
				inst := 1 / dt
				if u.fpsEst == 0 {
					u.fpsEst = inst
				} else {
					u.fpsEst = u.fpsEst*0.9 + inst*0.1
				}
			}
		}
		u.lastFrameAt = now
		u.lastErr = ""
		u.lastFrame = frame
		u.mu.Unlock()
		u.broadcast(frame)
	}
}

// setLastErr records the most recent capture error for the status API.
func (u *UsbCameraStreamer) setLastErr(err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	if len(msg) > 200 {
		msg = msg[:200]
	}
	u.mu.Lock()
	u.lastErr = msg
	u.mu.Unlock()
}

// CameraStreamStatus is a point-in-time health snapshot of a streamer.
type CameraStreamStatus struct {
	Running   bool   `json:"running"`
	Live      bool   `json:"live"`
	FPS       int    `json:"fps"`
	Frames    int64  `json:"frames"`
	LastError string `json:"lastError"`
}

// Status returns the current health of the streamer.
func (u *UsbCameraStreamer) Status() CameraStreamStatus {
	u.mu.Lock()
	defer u.mu.Unlock()
	st := CameraStreamStatus{
		Running:   u.running,
		Frames:    u.frames,
		LastError: u.lastErr,
	}
	if !u.lastFrameAt.IsZero() && time.Since(u.lastFrameAt) < 5*time.Second {
		st.Live = true
		st.FPS = int(u.fpsEst + 0.5)
	}
	return st
}

// dbgf records a streamer event to the debug console ring buffer and the log.
func (u *UsbCameraStreamer) dbgf(format string, args ...any) {
	Debugf("streamer:"+u.cameraName, format, args...)
}

// broadcast sends a frame to all subscribers (non-blocking).
func (u *UsbCameraStreamer) broadcast(frame []byte) {
	u.mu.Lock()
	subs := make([]chan []byte, len(u.subscribers))
	copy(subs, u.subscribers)
	u.mu.Unlock()

	for _, sub := range subs {
		select {
		case sub <- frame:
		default:
			// Drop frame if subscriber is slow
		}
	}
}

// buildStreamArgs constructs ffmpeg arguments to read from the USB camera
// and output MJPEG frames to stdout at a reasonable quality for streaming.
// tryAlt indicates whether to use fallback args (for virtual cameras like OBS).
func (u *UsbCameraStreamer) buildStreamArgs() []string {
	return u.buildStreamArgsInternal(false)
}

func (u *UsbCameraStreamer) buildAltStreamArgs() []string {
	return u.buildStreamArgsInternal(true)
}

// ffmpegVFFilters returns output video filters for the configured image
// adjustments (brightness, flip), or nil when none are set.
func (u *UsbCameraStreamer) ffmpegVFFilters() []string {
	var filters []string
	if u.config.Brightness != 0 {
		filters = append(filters, fmt.Sprintf("eq=brightness=%.2f", u.config.Brightness))
	}
	switch u.config.Flip {
	case "horizontal":
		filters = append(filters, "hflip")
	case "vertical":
		filters = append(filters, "vflip")
	case "both":
		filters = append(filters, "hflip,vflip")
	case "90":
		filters = append(filters, "transpose=1")
	case "270":
		filters = append(filters, "transpose=2")
	}
	return filters
}

// appendVFOpt appends -vf <filters> to args when any image adjustments are set.
func appendVFOpt(args []string, filters []string) []string {
	if len(filters) > 0 {
		return append(args, "-vf", strings.Join(filters, ","))
	}
	return args
}

// buildMinimalStreamArgs returns args with no resolution/format constraints.
// Used as a last-resort fallback for virtual cameras (e.g. OBS Virtual Camera)
// that only support specific resolutions and don't support MJPEG input.
func (u *UsbCameraStreamer) buildMinimalStreamArgs() []string {
	cam := &u.config

	switch runtime.GOOS {
	case "windows":
		device := cam.DeviceLabel
		if device == "" {
			device = cam.DeviceID
		}
		if idx := strings.LastIndex(device, " ("); idx > 0 && strings.HasSuffix(device, ")") {
			suffix := device[idx+2 : len(device)-1]
			if strings.Contains(suffix, ":") && len(suffix) <= 20 {
				device = device[:idx]
			}
		}
		args := []string{
			"-f", "dshow",
			"-rtbufsize", "100M",
			"-i", "video=" + device,
		}
		args = appendVFOpt(args, u.ffmpegVFFilters())
		return append(args,
			"-c:v", "mjpeg",
			"-q:v", "5",
			"-f", "mjpeg",
			"-",
		)
	case "linux":
		device := cam.DeviceID
		if device == "" {
			device = "/dev/video0"
		}
		args := []string{
			"-f", "v4l2",
			"-i", device,
		}
		args = appendVFOpt(args, u.ffmpegVFFilters())
		return append(args,
			"-c:v", "mjpeg",
			"-q:v", "5",
			"-f", "mjpeg",
			"-",
		)
	case "darwin":
		device := cam.DeviceID
		if device == "" {
			device = "0"
		}
		args := []string{
			"-f", "avfoundation",
			"-i", device,
		}
		args = appendVFOpt(args, u.ffmpegVFFilters())
		return append(args,
			"-c:v", "mjpeg",
			"-q:v", "5",
			"-f", "mjpeg",
			"-",
		)
	default:
		return []string{
			"-i", "dummy",
			"-f", "mjpeg",
			"-",
		}
	}
}

func (u *UsbCameraStreamer) buildStreamArgsInternal(alt bool) []string {
	cam := &u.config
	fps := 30 // streaming fps for live view

	switch runtime.GOOS {
	case "windows":
		device := cam.DeviceLabel
		if device == "" {
			device = cam.DeviceID
		}
		// Strip VID:PID suffix for dshow
		if idx := strings.LastIndex(device, " ("); idx > 0 && strings.HasSuffix(device, ")") {
			suffix := device[idx+2 : len(device)-1]
			if strings.Contains(suffix, ":") && len(suffix) <= 20 {
				device = device[:idx]
			}
		}
		args := []string{
			"-f", "dshow",
			"-rtbufsize", "100M",
			"-framerate", fmt.Sprintf("%d", fps),
		}
		if !alt {
			// Real USB cameras often support MJPEG for higher fps/lower bandwidth
			args = append(args, "-video_size", "1280x720", "-vcodec", "mjpeg")
		} else {
			// Fallback for virtual cameras (OBS, etc.) that don't support MJPEG
			// Try 640x480 which is more universally supported
			args = append(args, "-video_size", "640x480")
		}
		args = append(args, "-i", "video="+device)
		args = appendVFOpt(args, u.ffmpegVFFilters())
		args = append(args,
			"-c:v", "mjpeg",
			"-q:v", "5",
			"-f", "mjpeg",
			"-",
		)
		return args
	case "linux":
		device := cam.DeviceID
		if device == "" {
			device = "/dev/video0"
		}
		args := []string{
			"-f", "v4l2",
			"-framerate", fmt.Sprintf("%d", fps),
		}
		if !alt {
			args = append(args, "-video_size", "1280x720", "-input_format", "mjpeg")
		} else {
			args = append(args, "-video_size", "640x480")
		}
		args = append(args, "-i", device)
		args = appendVFOpt(args, u.ffmpegVFFilters())
		args = append(args,
			"-c:v", "mjpeg",
			"-q:v", "5",
			"-f", "mjpeg",
			"-",
		)
		return args
	case "darwin":
		device := cam.DeviceID
		if device == "" {
			device = "0"
		}
		args := []string{
			"-f", "avfoundation",
			"-framerate", fmt.Sprintf("%d", fps),
		}
		if !alt {
			args = append(args, "-video_size", "1280x720")
		} else {
			args = append(args, "-video_size", "640x480")
		}
		args = append(args, "-i", device)
		args = appendVFOpt(args, u.ffmpegVFFilters())
		args = append(args,
			"-c:v", "mjpeg",
			"-q:v", "5",
			"-f", "mjpeg",
			"-",
		)
		return args
	default:
		return []string{
			"-i", "dummy",
			"-f", "mjpeg",
			"-",
		}
	}
}

// readJpegFrame reads a single JPEG frame from a bufio.Reader.
// JPEG starts with 0xFF 0xD8 and ends with 0xFF 0xD9.
func readJpegFrame(r *bufio.Reader) ([]byte, error) {
	// Read until we find JPEG SOI marker (0xFF 0xD8)
	for {
		b, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		if b != 0xFF {
			continue
		}
		b2, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		if b2 == 0xD8 {
			break
		}
		// Not SOI, continue searching
	}

	// We found SOI (0xFF 0xD8), now read until EOI (0xFF 0xD9)
	var buf bytes.Buffer
	buf.WriteByte(0xFF)
	buf.WriteByte(0xD8)

	for {
		b, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		buf.WriteByte(b)

		if b == 0xFF {
			b2, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			buf.WriteByte(b2)

			if b2 == 0xD9 {
				// End of JPEG frame
				return buf.Bytes(), nil
			}
		}
	}
}

// ─── UsbStreamerManager manages all USB camera streamers ──────────────────────

// UsbStreamerManager manages the lifecycle of UsbCameraStreamer instances.
type UsbStreamerManager struct {
	mu        sync.RWMutex
	streamers map[string]*UsbCameraStreamer
}

// NewUsbStreamerManager creates a new manager.
func NewUsbStreamerManager() *UsbStreamerManager {
	return &UsbStreamerManager{
		streamers: make(map[string]*UsbCameraStreamer),
	}
}

// StartStream creates and starts a streamer for the given camera.
func (m *UsbStreamerManager) StartStream(cam *CameraConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.streamers[cam.ID]; ok {
		if existing.IsRunning() {
			return
		}
	}

	streamer := NewUsbCameraStreamer(cam)
	m.streamers[cam.ID] = streamer
	streamer.Start()
	sensor := ""
	if cam.Sensor != "" && cam.Sensor != "auto" {
		sensor = " sensor=" + cam.Sensor
	}
	Debugf("usbmgr", "started stream for camera %q (type=%s, id=%s%s)", cam.Name, cam.Type, cam.ID, sensor)
}

// StopStream stops and removes the streamer for the given camera ID.
func (m *UsbStreamerManager) StopStream(cameraID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if streamer, ok := m.streamers[cameraID]; ok {
		streamer.Stop()
		delete(m.streamers, cameraID)
		Debugf("usbmgr", "stopped stream for camera %s", cameraID)
	}
}

// GetStream returns the streamer for the given camera ID, or nil if not found.
func (m *UsbStreamerManager) GetStream(cameraID string) *UsbCameraStreamer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.streamers[cameraID]
}

// StopAll stops all active streamers.
func (m *UsbStreamerManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, streamer := range m.streamers {
		streamer.Stop()
		delete(m.streamers, id)
	}
	Debugf("usbmgr", "stopped all streams")
}

// StartAllFromSettings starts streamers for all enabled USB and Pi cameras in the camera store.
func (m *UsbStreamerManager) StartAllFromSettings(store *Store) {
	settings := store.GetSettings()
	for _, cam := range settings.Cameras {
		if cam.Type == "usb" || cam.Type == "rpicam" {
			m.StartStream(&cam)
		}
	}
}
