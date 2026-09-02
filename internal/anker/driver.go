package anker

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/lucas/openpolyprint/internal/anker/proto/config"
	"github.com/lucas/openpolyprint/internal/anker/proto/mqtt"
	"github.com/lucas/openpolyprint/internal/anker/proto/pppp"
	"github.com/lucas/openpolyprint/internal/printers"
)

//go:embed ankermake-mqtt.crt
var caCert []byte

// Driver implements the printers.Driver interface for AnkerMake M5 / M5C printers.
type Driver struct {
	printer    config.Printer
	account    *config.Account
	api        *pppp.PPPPApi
	apiMu      sync.Mutex
	mqttClient *mqtt.AnkerMQTTClient
	mqttCtx    context.Context
	mqttCancel context.CancelFunc
	pstate     *PrinterState
}

var _ printers.Driver = (*Driver)(nil)

// NewDriver builds an AnkerMake driver from a config.Printer entry and optional account.
func NewDriver(p config.Printer, account *config.Account) *Driver {
	return &Driver{
		printer: p,
		account: account,
		pstate:  NewPrinterState(),
	}
}

// PrinterID returns the printer's stable ID.
func (d *Driver) PrinterID() string { return d.printer.ID }

// Name returns the human-readable printer name.
func (d *Driver) Name() string { return d.printer.Name }

// Type returns the model string, e.g. "M5".
func (d *Driver) Type() string { return d.printer.Model }

// Connect opens the PPPP LAN connection and, if cloud credentials are available, MQTT.
// If the LAN connection fails, MQTT is still attempted so the printer can be
// controlled/monitored through the cloud.
func (d *Driver) Connect(ctx context.Context) error {
	hasConnection := false

	log.Printf("anker connect %s: ip=%q duid=%q sn=%q", d.printer.Name, d.printer.IPAddr, d.printer.P2PDUID, d.printer.SN)

	if err := d.connectPPPP(ctx); err != nil {
		log.Printf("anker pppp for %s: %v", d.printer.Name, err)
	}
	if d.api != nil {
		hasConnection = true
	}

	if d.account != nil {
		if err := d.connectMQTT(ctx); err != nil {
			log.Printf("anker mqtt connect for %s: %v", d.printer.Name, err)
		} else {
			d.mqttCtx, d.mqttCancel = context.WithCancel(context.Background())
			go d.mqttPoll(d.mqttCtx)
			hasConnection = true
		}
	}

	if !hasConnection {
		return fmt.Errorf("no local or cloud connection available")
	}
	return nil
}

// connectPPPP establishes the PPPP LAN connection. If the printer's IP is
// empty, it discovers it via LAN broadcast. Safe to call multiple times.
func (d *Driver) connectPPPP(ctx context.Context) error {
	if d.api != nil {
		return nil // already connected
	}
	if d.printer.P2PDUID == "" {
		return fmt.Errorf("no P2P DUID in config")
	}

	duid, err := pppp.DuidFromString(d.printer.P2PDUID)
	if err != nil {
		return fmt.Errorf("parse DUID: %w", err)
	}

	ipAddr := d.printer.IPAddr
	// If IP is empty, discover it via LAN broadcast
	if ipAddr == "" {
		log.Printf("anker pppp for %s: IP empty, discovering via LAN broadcast...", d.printer.Name)
		discoveredIP, err := pppp.DiscoverPrinterIP(duid, 5*time.Second)
		if err != nil {
			return fmt.Errorf("LAN discovery failed: %w", err)
		}
		ipAddr = discoveredIP
		log.Printf("anker pppp for %s: discovered printer IP %s", d.printer.Name, ipAddr)
	}

	api, err := pppp.NewPPPPApiLAN(&duid, ipAddr)
	if err != nil {
		return fmt.Errorf("open PPPP: %w", err)
	}

	if err := api.ConnectLanSearch(); err != nil {
		_ = api.Close()
		return fmt.Errorf("connect LAN search: %w", err)
	}

	go api.Run()
	deadline := time.Now().Add(30 * time.Second)
	for api.State() != pppp.StateConnected && time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			api.Stop()
			_ = api.Close()
			return ctx.Err()
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}

	if api.State() == pppp.StateConnected {
		log.Printf("anker pppp connected to %s at %s", d.printer.Name, ipAddr)
		d.api = api
		return nil
	}

	api.Stop()
	_ = api.Close()
	return fmt.Errorf("timed out waiting for state=Connected (final state=%s)", api.State())
}

func (d *Driver) connectMQTT(ctx context.Context) error {
	client, err := mqtt.NewAnkerMQTTClient(
		d.printer.SN,
		d.account.MQTTUsername(),
		d.account.MQTTPassword(),
		d.printer.MQTTKey,
		caCert,
		false,
	)
	if err != nil {
		return fmt.Errorf("new mqtt client: %w", err)
	}

	server := "make-mqtt-eu.ankermake.com"
	if d.account.Region == "us" {
		server = "make-mqtt.ankermake.com"
	}
	port := 8789

	if err := client.Connect(server, port, 10*time.Second); err != nil {
		return fmt.Errorf("mqtt connect: %w", err)
	}

	d.mqttClient = client
	d.pstate.SetConnected(true)
	return nil
}

func (d *Driver) mqttPoll(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if d.mqttClient == nil {
				continue
			}
			msgs := d.mqttClient.Fetch(100 * time.Millisecond)
			for _, msg := range msgs {
				for _, obj := range msg.Body {
					if ct, ok := obj["commandType"].(float64); ok && (ct == 1003 || ct == 1004) {
						log.Printf("anker mqtt %s: temp commandType=%v", d.printer.Name, ct)
					}
					d.pstate.UpdateFromMQTT(obj)
				}
			}
		}
	}
}

// Disconnect closes the PPPP and MQTT sessions.
func (d *Driver) Disconnect() error {
	if d.mqttCancel != nil {
		d.mqttCancel()
		d.mqttCancel = nil
	}
	if d.mqttClient != nil {
		d.mqttClient.Disconnect()
		d.mqttClient = nil
	}
	if d.pstate != nil {
		d.pstate.SetConnected(false)
	}
	if d.api != nil {
		d.api.Stop()
		_ = d.api.Close()
		d.api = nil
	}
	return nil
}

// Status returns the current state of the printer.
func (d *Driver) Status() (printers.Status, error) {
	ps := d.pstate.Snapshot()
	s := printers.Status{
		ID:            d.printer.ID,
		Name:          d.printer.Name,
		Type:          d.printer.Model,
		Online:        (d.api != nil && d.api.State() == pppp.StateConnected) || ps["connected"].(bool),
		State:         "idle",
		StatusText:    "Idle",
		Temps:         printers.Temps{Nozzle: ps["nozzleTemp"].(float64), Bed: ps["bedTemp"].(float64), TargetNozzle: ps["setNozzleTemp"].(float64), TargetBed: ps["setBedTemp"].(float64)},
		Progress:      int(ps["progress"].(float64) / 100),
		CurrentFile:   ps["fileName"].(string),
		RemainingTime: formatDurationSeconds(ps["timeRemaining"].(int64)),
		UpdatedAt:     time.Now(),
		LayerNum:      ps["layerNum"].(int),
		LayerCount:    ps["layerCount"].(int),
	}

	if s.Online {
		s.State = "connected"
		if raw, ok := ps["state"].(string); ok && raw != "" {
			s.State = raw
			s.StatusText = humanizeState(raw)
		}
		if progress, ok := ps["progress"].(float64); ok && progress > 0 && progress < 10000 {
			s.StatusText = "Printing"
		} else if progress >= 10000 {
			// Print reached 100% — report as Finished so history records
			// it as Success. The printer keeps the file loaded after
			// completion, so we can't rely on "Idle" + empty CurrentFile.
			s.StatusText = "Finished"
		} else if s.StatusText == "Idle" {
			// Check if printer is heating — target temps set but not reached yet
			nozzleTarget := ps["setNozzleTemp"].(float64)
			bedTarget := ps["setBedTemp"].(float64)
			nozzleCurrent := ps["nozzleTemp"].(float64)
			bedCurrent := ps["bedTemp"].(float64)
			if (nozzleTarget > 0 && nozzleCurrent < nozzleTarget-5) ||
				(bedTarget > 0 && bedCurrent < bedTarget-5) {
				s.StatusText = "Heating"
				s.State = "heating"
			}
		}
	} else {
		s.StatusText = "Offline"
		s.State = "offline"
	}

	return s, nil
}

func humanizeState(state string) string {
	switch state {
	case "idle":
		return "Idle"
	case "printing":
		return "Printing"
	case "paused":
		return "Paused"
	case "heating":
		return "Heating"
	default:
		return "Idle"
	}
}

func formatDurationSeconds(secs int64) string {
	if secs <= 0 {
		return ""
	}
	h := secs / 3600
	m := (secs % 3600) / 60
	if h > 0 {
		return fmt.Sprintf("%dh %02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

// sendCommand builds and sends an MQTT command message.
func (d *Driver) sendCommand(cmd mqtt.MqttMsgType, fields map[string]any) error {
	if d.mqttClient == nil {
		return errors.New("mqtt not available (offline or missing account)")
	}
	if !d.mqttClient.IsConnected() {
		return errors.New("mqtt not connected")
	}

	msg := map[string]any{
		"commandType": int(cmd),
	}
	for k, v := range fields {
		msg[k] = v
	}
	if err := d.mqttClient.Command(msg); err != nil {
		return err
	}
	return nil
}

// PausePrint pauses the active print.
func (d *Driver) PausePrint(ctx context.Context) error {
	return d.sendCommand(mqtt.CmdPrintControl, map[string]any{"control": 0})
}

// StopPrint stops the active print.
func (d *Driver) StopPrint(ctx context.Context) error {
	return d.sendCommand(mqtt.CmdPrintControl, map[string]any{"control": 2})
}

// Home homes all axes.
func (d *Driver) Home(ctx context.Context) error {
	return d.sendCommand(mqtt.CmdMoveZero, map[string]any{"value": 0})
}

// Preheat sets nozzle and bed target temperatures.
func (d *Driver) Preheat(ctx context.Context, nozzle, bed float64) error {
	return d.sendCommand(mqtt.CmdPreheatConfig, map[string]any{
		"nozzle_temp": nozzle,
		"bed_temp":    bed,
	})
}

// Cooldown turns off heaters.
func (d *Driver) Cooldown(ctx context.Context) error {
	return d.Preheat(ctx, 0, 0)
}

// AutoLevel starts automatic bed leveling.
func (d *Driver) AutoLevel(ctx context.Context) error {
	return d.sendCommand(mqtt.CmdAutoLeveling, map[string]any{"value": 1})
}

// SendGCode sends one or more raw G-code lines. Multi-line commands
// (separated by \n) are split and sent as individual MQTT messages.
func (d *Driver) SendGCode(ctx context.Context, command string) error {
	lines := strings.Split(command, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		if err := d.sendCommand(mqtt.CmdGcodeCommand, map[string]any{"gcode": line}); err != nil {
			return err
		}
	}
	return nil
}

// MoveAxis moves an axis by a relative distance using the native Move Step
// command (0x0400). This is more reliable than sending raw G-code for jog
// controls because the printer firmware handles it directly.
func (d *Driver) MoveAxis(ctx context.Context, axis string, distance float64, speed float64) error {
	return d.sendCommand(mqtt.CmdMoveStep, map[string]any{
		"axis":  axis,
		"step":  distance,
		"speed": speed,
	})
}

// UploadGCode sends a G-code file to the printer via the PPPP file transfer
// protocol. If the PPPP LAN connection is not available, it attempts to
// reconnect before returning an error.
func (d *Driver) UploadGCode(ctx context.Context, filename string, data []byte) error {
	d.apiMu.Lock()
	defer d.apiMu.Unlock()

	if d.api == nil {
		log.Printf("[pppp] no connection for upload, attempting reconnect to %s...", d.printer.Name)
		// Try to reconnect — this will run LAN discovery if IP is empty
		if err := d.connectPPPP(ctx); err != nil {
			return fmt.Errorf("pppp not available: reconnect failed for %s: %v (ip=%q duid=%q)", d.printer.Name, err, d.printer.IPAddr, d.printer.P2PDUID)
		}
		if d.api == nil {
			return fmt.Errorf("pppp not available: %s connected via MQTT only, PPPP LAN connection failed (ip=%q duid=%q). Printer may be offline or on a different network.", d.printer.Name, d.printer.IPAddr, d.printer.P2PDUID)
		}
		log.Printf("[pppp] reconnected successfully to %s", d.printer.Name)
	}

	cleanName := pppp.SanitizeFilename(filename)
	userID := "-"
	machineID := "-"
	if d.account != nil {
		userID = d.account.UserID
		machineID = d.printer.SN
	}

	// Build file upload info (CSV metadata)
	info := pppp.FileUploadInfoFromData(data, cleanName, "OpenPolyPrint", userID, machineID, 0)

	// Step 1: Send P2P_SEND_FILE command via XZYH to initiate file transfer.
	// The Python reference sends a 16-char UUID string as the data.
	uuidData := []byte("0123456789abcdef") // 16 chars, like uuid4()[:16]
	d.api.SendXzyh(uuidData, pppp.P2PCmdP2PSendFile, 0, false)

	// Step 2: Send FTBegin with file metadata.
	// The Python reference sends bytes(fui) + b"\x00" (two null terminators).
	beginData := append(info.Bytes(), 0)
	d.api.SendAabb(beginData, 0, 0, pppp.FTBegin, 1, true)

	// Step 3: Send file data in 32KB chunks, waiting for reply on each.
	const chunkSize = 32 * 1024
	pos := uint32(0)
	for pos < uint32(len(data)) {
		end := pos + chunkSize
		if end > uint32(len(data)) {
			end = uint32(len(data))
		}
		chunk := data[pos:end]
		if _, err := d.api.AabbRequest(chunk, pppp.FTData, pos, 1, true); err != nil {
			return fmt.Errorf("file upload data at offset %d: %w", pos, err)
		}
		pos = end
	}

	// Step 4: Send FTEnd to complete the transfer (this also starts the print).
	if _, err := d.api.AabbRequest(nil, pppp.FTEnd, uint32(len(data)), 1, true); err != nil {
		return fmt.Errorf("file upload end: %w", err)
	}

	return nil
}

// StartPrint tells the printer to start printing a file that has already been
// uploaded. Uses MQTT CmdPrintSchedule to select and start the file.
func (d *Driver) StartPrint(ctx context.Context, filename string) error {
	cleanName := pppp.SanitizeFilename(filename)
	// CmdPrintSchedule (0x03e9) is used to start a print from a file on the
	// printer's storage. The "schedule" field tells the printer to start now.
	return d.sendCommand(mqtt.CmdPrintSchedule, map[string]any{
		"file_name": cleanName,
		"schedule":  0, // 0 = start immediately
	})
}
