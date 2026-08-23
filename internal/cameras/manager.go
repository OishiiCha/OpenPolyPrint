package cameras

// Manager owns the camera store and USB/CSI streamers.
type Manager struct {
	store      *Store
	streamers  *UsbStreamerManager
	records    *RecordManager
	timelapses *TimelapseManager
	names      *NamesStore
}

// NewManager creates a Manager, loading saved settings but not starting any cameras automatically.
// Cameras are only started when the user adds them or requests a preview.
func NewManager(configDir string) *Manager {
	st := NewStore(configDir)
	return &Manager{
		store:      st,
		streamers:  NewUsbStreamerManager(),
		records:    NewRecordManager(),
		timelapses: NewTimelapseManager(),
		names:      NewNamesStore(),
	}
}

// Store returns the underlying camera store.
func (m *Manager) Store() *Store { return m.store }

// Streamers returns the underlying streamer manager.
func (m *Manager) Streamers() *UsbStreamerManager { return m.streamers }

// Records returns the recording manager.
func (m *Manager) Records() *RecordManager { return m.records }

// Timelapses returns the timelapse manager.
func (m *Manager) Timelapses() *TimelapseManager { return m.timelapses }

// Names returns the recording display-name store.
func (m *Manager) Names() *NamesStore { return m.names }
