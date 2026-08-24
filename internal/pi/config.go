// Package pi provides Raspberry Pi GPIO control and DHT11 filament-sensor support.
package pi

// FilamentSensor represents a single DHT11 sensor in a filament storage box.
// Up to 5 sensors are supported. Each reads temperature and humidity.
type FilamentSensor struct {
	ID           int    `json:"id"`
	Enabled      bool   `json:"enabled"`
	Name         string `json:"name"`         // e.g. "Box 1"
	GPIOPin      int    `json:"gpioPin"`      // BCM pin for DHT11 data line
	FilamentType string `json:"filamentType"` // PLA, PLA+, PETG, PETG+, ABS, TPU, etc
	Color        string `json:"color"`        // hex color e.g. #FF6B6B
}

// Settings holds Raspberry Pi-specific settings such as GPIO light relay control
// and filament box DHT11 sensors.
type Settings struct {
	LightRelayEnabled bool             `json:"lightRelayEnabled"`
	LightRelayGPIO    int              `json:"lightRelayGpio"`
	LightRelayOn      bool             `json:"lightRelayOn"`
	FilamentSensors   []FilamentSensor `json:"filamentSensors"`
}

// DefaultSettings returns an empty default Settings struct.
func DefaultSettings() Settings {
	return Settings{
		LightRelayGPIO:  17,
		FilamentSensors: []FilamentSensor{},
	}
}
