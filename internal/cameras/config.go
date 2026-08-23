package cameras

import "time"

// CameraConfig represents a single external camera configuration.
type CameraConfig struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Type        string  `json:"type"` // "stream", "usb" or "rpicam"
	PrinterID   string  `json:"printerId,omitempty"`
	Enabled     bool    `json:"enabled"`
	URL         string  `json:"url,omitempty"`
	DeviceID    string  `json:"deviceId,omitempty"`
	DeviceLabel string  `json:"deviceLabel,omitempty"`
	Sensor      string  `json:"sensor,omitempty"`     // Pi camera sensor (e.g. "imx219"), for rpicam type
	Brightness  float64 `json:"brightness,omitempty"` // image brightness, -1..1 (0 = camera default)
	Flip        string  `json:"flip,omitempty"`       // "", "horizontal", "vertical", "both", "90", "270"
}

// CameraSettings holds all camera-related settings persisted server-side.
type CameraSettings struct {
	Cameras         []CameraConfig    `json:"cameras"`
	CameraModule    bool              `json:"cameraModule"`
	BuiltinDisabled map[string]bool   `json:"builtinDisabled"`
	RecSettings     RecordingSettings `json:"recSettings"`
	ActiveCameraIdx int               `json:"activeCameraIdx"`
	CameraLayout    string            `json:"cameraLayout"`
}

// RecordingSettings holds server-side recording configuration.
type RecordingSettings struct {
	Framerate      int    `json:"framerate"`
	Resolution     int    `json:"resolution"`
	Bitrate        int    `json:"bitrate"`
	Format         string `json:"format"` // "mkv", "webm", "auto"
	Audio          bool   `json:"audio"`
	AutoRecord     bool   `json:"autoRecord"`
	AutoRecordStop bool   `json:"autoRecordStop"`
}

// RecordingEntry tracks a single camera's recording process.
type RecordingEntry struct {
	CameraID   string
	CameraName string
	Filename   string
	Format     string
	StartedAt  time.Time
	Stderr     string
}

// RecordingState tracks the current recording status (server-side).
type RecordingState struct {
	Recording  bool      `json:"recording"`
	CameraID   string    `json:"cameraId"`
	CameraName string    `json:"cameraName"`
	Filename   string    `json:"filename"`
	StartedAt  time.Time `json:"startedAt"`
	Format     string    `json:"format"`
}

// DefaultRecordingSettings returns sensible defaults.
func DefaultRecordingSettings() RecordingSettings {
	return RecordingSettings{
		Framerate:      30,
		Resolution:     720,
		Bitrate:        2500000,
		Format:         "mkv",
		Audio:          true,
		AutoRecord:     false,
		AutoRecordStop: true,
	}
}
