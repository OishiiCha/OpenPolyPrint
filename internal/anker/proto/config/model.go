// Package config implements the configuration model and management for ankerctl.
//
// This file defines the Account, Printer, and Config data structures
// used to persist user credentials and printer information.
package config

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// Account represents the user's account credentials.
type Account struct {
	AuthToken string `json:"auth_token"`
	Region    string `json:"region"`
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
}

// MQTTUsername returns the MQTT username for this account.
func (a *Account) MQTTUsername() string {
	return fmt.Sprintf("eufy_%s", a.UserID)
}

// MQTTPassword returns the MQTT password for this account.
func (a *Account) MQTTPassword() string {
	return a.Email
}

// Printer represents a single printer configuration entry.
type Printer struct {
	ID         string    `json:"id"`
	SN         string    `json:"sn"`
	Name       string    `json:"name"`
	Model      string    `json:"model"`
	CreateTime time.Time `json:"create_time"`
	UpdateTime time.Time `json:"update_time"`
	WifiMAC    string    `json:"wifi_mac"`
	IPAddr     string    `json:"ip_addr"`
	MQTTKey    []byte    `json:"mqtt_key"` // stored as hex in JSON
	APIHosts   []string  `json:"api_hosts"`
	P2PHosts   []string  `json:"p2p_hosts"`
	P2PDUID    string    `json:"p2p_duid"`
	P2PKey     string    `json:"p2p_key"`
}

// Config represents the full ankerctl configuration.
type Config struct {
	Account  *Account  `json:"account"`
	Printers []Printer `json:"printers"`
}

// HasPrinters returns true if the config has any printers.
func (c *Config) HasPrinters() bool {
	return len(c.Printers) > 0
}

// MarshalJSON implements custom JSON marshaling for Config to handle
// the hex encoding of MQTTKey and timestamp formatting.
func (p Printer) MarshalJSON() ([]byte, error) {
	type alias struct {
		ID         string   `json:"id"`
		SN         string   `json:"sn"`
		Name       string   `json:"name"`
		Model      string   `json:"model"`
		CreateTime float64  `json:"create_time"`
		UpdateTime float64  `json:"update_time"`
		WifiMAC    string   `json:"wifi_mac"`
		IPAddr     string   `json:"ip_addr"`
		MQTTKey    string   `json:"mqtt_key"`
		APIHosts   []string `json:"api_hosts"`
		P2PHosts   []string `json:"p2p_hosts"`
		P2PDUID    string   `json:"p2p_duid"`
		P2PKey     string   `json:"p2p_key"`
	}

	return json.Marshal(alias{
		ID:         p.ID,
		SN:         p.SN,
		Name:       p.Name,
		Model:      p.Model,
		CreateTime: float64(p.CreateTime.Unix()),
		UpdateTime: float64(p.UpdateTime.Unix()),
		WifiMAC:    p.WifiMAC,
		IPAddr:     p.IPAddr,
		MQTTKey:    hex.EncodeToString(p.MQTTKey),
		APIHosts:   p.APIHosts,
		P2PHosts:   p.P2PHosts,
		P2PDUID:    p.P2PDUID,
		P2PKey:     p.P2PKey,
	})
}

// UnmarshalJSON implements custom JSON unmarshaling for Printer.
func (p *Printer) UnmarshalJSON(data []byte) error {
	type alias struct {
		ID         string   `json:"id"`
		SN         string   `json:"sn"`
		Name       string   `json:"name"`
		Model      string   `json:"model"`
		CreateTime float64  `json:"create_time"`
		UpdateTime float64  `json:"update_time"`
		WifiMAC    string   `json:"wifi_mac"`
		IPAddr     string   `json:"ip_addr"`
		MQTTKey    string   `json:"mqtt_key"`
		APIHosts   []string `json:"api_hosts"`
		P2PHosts   []string `json:"p2p_hosts"`
		P2PDUID    string   `json:"p2p_duid"`
		P2PKey     string   `json:"p2p_key"`
	}

	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}

	p.ID = a.ID
	p.SN = a.SN
	p.Name = a.Name
	p.Model = a.Model
	p.CreateTime = time.Unix(int64(a.CreateTime), 0)
	p.UpdateTime = time.Unix(int64(a.UpdateTime), 0)
	p.WifiMAC = a.WifiMAC
	p.IPAddr = a.IPAddr

	mqttKey, err := hex.DecodeString(a.MQTTKey)
	if err != nil {
		return fmt.Errorf("decode mqtt_key hex: %w", err)
	}
	p.MQTTKey = mqttKey
	p.APIHosts = a.APIHosts
	p.P2PHosts = a.P2PHosts
	p.P2PDUID = a.P2PDUID
	p.P2PKey = a.P2PKey

	return nil
}
