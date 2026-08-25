package anker

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"
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

	if d.printer.IPAddr != "" && d.printer.P2PDUID != "" {
		duid, err := pppp.DuidFromString(d.printer.P2PDUID)
		if err == nil {
			api, err := pppp.NewPPPPApiLAN(&duid, d.printer.IPAddr)
			if err == nil {
				if err := api.ConnectLanSearch(); err == nil {
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
						d.api = api
						hasConnection = true
					} else {
						api.Stop()
						_ = api.Close()
					}
				} else {
					_ = api.Close()
					log.Printf("anker pppp connect for %s: %v", d.printer.Name, err)
				}
			} else {
				log.Printf("anker pppp open for %s: %v", d.printer.Name, err)
			}
		} else {
			log.Printf("anker pppp duid for %s: %v", d.printer.Name, err)
		}
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
	return d.sendCommand(mqtt.CmdPrintControl, map[string]any{"value": 0})
}

// StopPrint stops the active print.
func (d *Driver) StopPrint(ctx context.Context) error {
	return d.sendCommand(mqtt.CmdPrintControl, map[string]any{"value": 2})
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

// SendGCode sends one raw G-code line.
func (d *Driver) SendGCode(ctx context.Context, command string) error {
	return d.sendCommand(mqtt.CmdGcodeCommand, map[string]any{"command": command})
}
