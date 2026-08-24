// Package mqtt implements the MQTT client API for AnkerMake M5 printers.
//
// This file defines the MQTT API client with subscribe/publish/await-response logic.
// It is the Go equivalent of libflagship/mqttapi.py from the Python implementation.
package mqtt

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// AnkerMQTTClient is the MQTT client for communicating with AnkerMake M5 printers.
type AnkerMQTTClient struct {
	client    mqtt.Client
	printersn string
	key       []byte
	guid      string
	queue     []MQTTMessage
	mu        sync.Mutex
	connected bool

	username  string
	password  string
	tlsConfig *tls.Config
}

// MQTTMessage represents a received MQTT message with its parsed payload.
type MQTTMessage struct {
	Topic   string
	Payload []byte
	Body    []map[string]any
	Pkt     MqttMsg
}

// NewAnkerMQTTClient creates a new MQTT client configured but not yet connected.
func NewAnkerMQTTClient(printersn, username, password string, key []byte, caCert []byte, verify bool) (*AnkerMQTTClient, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: !verify,
	}
	if caCert != nil {
		pool, err := newCertPool(caCert)
		if err != nil {
			return nil, fmt.Errorf("parse CA cert: %w", err)
		}
		tlsConfig.RootCAs = pool
	}

	c := &AnkerMQTTClient{
		printersn: printersn,
		key:       key,
		guid:      newUUID(),
		username:  username,
		password:  password,
		tlsConfig: tlsConfig,
	}

	return c, nil
}

// Connect connects to the MQTT broker.
func (c *AnkerMQTTClient) Connect(server string, port int, timeout time.Duration) error {
	broker := fmt.Sprintf("ssl://%s:%d", server, port)

	opts := mqtt.NewClientOptions().
		AddBroker(broker).
		SetTLSConfig(c.tlsConfig).
		SetUsername(c.username).
		SetPassword(c.password).
		SetAutoReconnect(true).
		SetOnConnectHandler(func(client mqtt.Client) {
			c.mu.Lock()
			c.connected = true
			c.mu.Unlock()

			sn := c.printersn
			client.Subscribe(fmt.Sprintf("/phone/maker/%s/notice", sn), 0, c.handleMessage)
			client.Subscribe(fmt.Sprintf("/phone/maker/%s/command/reply", sn), 0, c.handleMessage)
			client.Subscribe(fmt.Sprintf("/phone/maker/%s/query/reply", sn), 0, c.handleMessage)
		}).
		SetConnectionLostHandler(func(client mqtt.Client, err error) {
			c.mu.Lock()
			c.connected = false
			c.mu.Unlock()
		})

	c.client = mqtt.NewClient(opts)

	token := c.client.Connect()
	if !token.WaitTimeout(timeout) {
		return fmt.Errorf("timeout connecting to MQTT broker")
	}
	if token.Error() != nil {
		return fmt.Errorf("connect to MQTT broker: %w", token.Error())
	}

	return nil
}

// handleMessage processes incoming MQTT messages.
func (c *AnkerMQTTClient) handleMessage(client mqtt.Client, msg mqtt.Message) {
	pkt, err := ParseMqttMsgEncrypted(msg.Payload(), c.key)
	if err != nil {
		log.Printf("MQTT: failed to parse message on topic %s: %v", msg.Topic(), err)
		return
	}

	var data any
	if err := json.Unmarshal(pkt.Data, &data); err != nil {
		log.Printf("MQTT: failed to unmarshal JSON: %v", err)
		return
	}

	var body []map[string]any
	switch v := data.(type) {
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				body = append(body, m)
			}
		}
	case map[string]any:
		body = []map[string]any{v}
	}

	c.mu.Lock()
	c.queue = append(c.queue, MQTTMessage{
		Topic:   msg.Topic(),
		Payload: msg.Payload(),
		Body:    body,
		Pkt:     pkt,
	})
	c.mu.Unlock()
}

// ParseMqttMsgEncrypted parses an encrypted MQTT message (with checksum and AES decryption).
func ParseMqttMsgEncrypted(payload, key []byte) (MqttMsg, error) {
	decrypted, err := mqttChecksumRemove(payload)
	if err != nil {
		return MqttMsg{}, fmt.Errorf("mqtt checksum: %w", err)
	}

	if len(decrypted) < 12 {
		return MqttMsg{}, fmt.Errorf("mqtt message too short: %d bytes", len(decrypted))
	}

	if decrypted[6] != 1 && decrypted[6] != 2 {
		return MqttMsg{}, fmt.Errorf("unsupported mqtt message format (expected 1 or 2, but found %d)", decrypted[6])
	}

	// M5 (m5=2): 64-byte header with timestamp + GUID + padding
	// M5C (m5=1): 24-byte header with just 12 bytes of padding (no timestamp, no GUID)
	headerSize := 64
	if decrypted[6] == 1 {
		headerSize = 24
	}

	if len(decrypted) < headerSize {
		return MqttMsg{}, fmt.Errorf("mqtt message too short: %d bytes (need %d)", len(decrypted), headerSize)
	}

	header := decrypted[:headerSize]
	aesData, err := mqttAESDecrypt(decrypted[headerSize:], key)
	if err != nil {
		return MqttMsg{}, fmt.Errorf("mqtt AES decrypt: %w", err)
	}

	msg, _, err := ParseMqttMsg(append(header, aesData...))
	if err != nil {
		return MqttMsg{}, err
	}

	// Verify size field (only for M5; M5C size field may differ)
	if decrypted[6] == 2 {
		expectedSize := len(decrypted) + 1
		if int(msg.Size) != expectedSize {
			return MqttMsg{}, fmt.Errorf("size mismatch: expected %d, got %d", expectedSize, msg.Size)
		}
	}

	return msg, nil
}

// PackEncrypted packs an MqttMsg with AES encryption and checksum.
func (msg *MqttMsg) PackEncrypted(key []byte) ([]byte, error) {
	encryptedData, err := mqttAESEncrypt(msg.Data, key)
	if err != nil {
		return nil, fmt.Errorf("mqtt AES encrypt: %w", err)
	}

	msg.Size = uint16(64 + len(encryptedData) + 1)
	header := msg.PackHeader()

	body := append(header, encryptedData...)
	checksummed := mqttChecksumAdd(body)

	return checksummed, nil
}

// Send sends a JSON command on a topic.
func (c *AnkerMQTTClient) Send(topic string, msg map[string]any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal mqtt message: %w", err)
	}

	pkt := NewMqttMsg(PktTypeSingle, 0, c.guid, data)
	payload, err := pkt.PackEncrypted(c.key)
	if err != nil {
		return fmt.Errorf("pack mqtt message: %w", err)
	}

	token := c.client.Publish(topic, 0, false, payload)
	if token.Error() != nil {
		return fmt.Errorf("publish mqtt message: %w", token.Error())
	}

	return nil
}

// Query sends a query message to the printer.
func (c *AnkerMQTTClient) Query(msg map[string]any) error {
	return c.Send(fmt.Sprintf("/device/maker/%s/query", c.printersn), msg)
}

// Command sends a command message to the printer.
func (c *AnkerMQTTClient) Command(msg map[string]any) error {
	return c.Send(fmt.Sprintf("/device/maker/%s/command", c.printersn), msg)
}

// Fetch retrieves queued messages with a timeout.
func (c *AnkerMQTTClient) Fetch(timeout time.Duration) []MQTTMessage {
	time.Sleep(timeout)
	return c.ClearQueue()
}

// ClearQueue returns all queued messages and clears the queue.
func (c *AnkerMQTTClient) ClearQueue() []MQTTMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := c.queue
	c.queue = nil
	return result
}

// AwaitResponse waits for a message of the specified command type.
func (c *AnkerMQTTClient) AwaitResponse(msgType uint16, timeout time.Duration) (map[string]any, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		if remaining < 0 {
			remaining = 0
		}

		msgs := c.Fetch(remaining)
		for _, m := range msgs {
			for _, obj := range m.Body {
				if ct, ok := obj["commandType"]; ok {
					if asFloat, ok := ct.(float64); ok && uint16(asFloat) == msgType {
						return obj, nil
					}
				}
			}
		}
	}

	return nil, fmt.Errorf("timeout waiting for message type 0x%04x", msgType)
}

// IsConnected returns whether the client is currently connected.
func (c *AnkerMQTTClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// Disconnect disconnects from the MQTT broker.
func (c *AnkerMQTTClient) Disconnect() {
	c.client.Disconnect(100)
}

// GUID returns the client's device GUID.
func (c *AnkerMQTTClient) GUID() string {
	return c.guid
}

// PrinterSN returns the printer serial number.
func (c *AnkerMQTTClient) PrinterSN() string {
	return c.printersn
}
