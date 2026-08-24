package config

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/lucas/openpolyprint/internal/anker/proto/crypto"
	"github.com/lucas/openpolyprint/internal/anker/proto/httpapi"
	"github.com/lucas/openpolyprint/internal/anker/proto/types"
)

// anyToString safely converts a map[string]any value to string,
// handling both string and float64 (JSON number) types.
func anyToString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	default:
		return ""
	}
}

func loadConfigFromAPI(authToken, region string, insecure bool) (*Config, error) {
	return loadConfigFromAPIWithUserID(authToken, region, "", insecure)
}

func loadConfigFromAPIWithUserID(authToken, region, userID string, insecure bool) (*Config, error) {
	reg := httpapi.Region(region)

	appapi, err := httpapi.NewAnkerHTTPAppApiV1(reg, authToken, !insecure)
	if err != nil {
		return nil, fmt.Errorf("create app API: %w", err)
	}
	appapi.UserID = userID

	ppapi, err := httpapi.NewAnkerHTTPPassportApiV1(reg, authToken, !insecure)
	if err != nil {
		return nil, fmt.Errorf("create passport API: %w", err)
	}
	ppapi.UserID = userID

	// Request profile
	profileData, err := ppapi.Profile()
	if err != nil {
		return nil, fmt.Errorf("request profile: %w", err)
	}

	var profile struct {
		UserID string `json:"user_id"`
		Email  string `json:"email"`
	}
	if err := json.Unmarshal(profileData, &profile); err != nil {
		return nil, fmt.Errorf("parse profile: %w", err)
	}

	cfg := &Config{
		Account: &Account{
			AuthToken: authToken,
			Region:    region,
			UserID:    profile.UserID,
			Email:     profile.Email,
		},
		Printers: []Printer{},
	}

	// Request printer list
	printersData, err := appapi.QueryFdmList()
	if err != nil {
		return nil, fmt.Errorf("query FDM list: %w", err)
	}

	var printers []map[string]any
	if err := json.Unmarshal(printersData, &printers); err != nil {
		return nil, fmt.Errorf("parse printer list: %w", err)
	}

	// Request DSK keys
	sns := make([]string, len(printers))
	for i, pr := range printers {
		sns[i] = pr["station_sn"].(string)
	}

	dskData, err := appapi.EquipmentGetDskKeys(sns, nil)
	if err != nil {
		return nil, fmt.Errorf("get DSK keys: %w", err)
	}

	var dskResp struct {
		DSKKeys []map[string]any `json:"dsk_keys"`
	}
	if err := json.Unmarshal(dskData, &dskResp); err != nil {
		return nil, fmt.Errorf("parse DSK keys: %w", err)
	}

	dskMap := make(map[string]map[string]any)
	for _, dsk := range dskResp.DSKKeys {
		dskMap[anyToString(dsk["station_sn"])] = dsk
	}

	// Sort printers by station_id
	sort.Slice(printers, func(i, j int) bool {
		return anyToString(printers[i]["station_id"]) < anyToString(printers[j]["station_id"])
	})

	for _, pr := range printers {
		stationSN := anyToString(pr["station_sn"])

		mqttKeyHex := anyToString(pr["secret_key"])
		mqttKey, err := types.UnHex(mqttKeyHex)
		if err != nil {
			return nil, fmt.Errorf("decode mqtt key for %s: %w", stationSN, err)
		}

		appConn := anyToString(pr["app_conn"])
		p2pConn := anyToString(pr["p2p_conn"])

		apiHosts, err := crypto.PPPPDecodeInitString(appConn)
		if err != nil {
			return nil, fmt.Errorf("decode app_conn for %s: %w", stationSN, err)
		}

		p2pHosts, err := crypto.PPPPDecodeInitString(p2pConn)
		if err != nil {
			return nil, fmt.Errorf("decode p2p_conn for %s: %w", stationSN, err)
		}

		var createTime, updateTime time.Time
		if ct, ok := pr["create_time"].(float64); ok {
			createTime = time.Unix(int64(ct), 0)
		}
		if ut, ok := pr["update_time"].(float64); ok {
			updateTime = time.Unix(int64(ut), 0)
		}

		dsk, ok := dskMap[stationSN]
		var dskKey string
		if ok {
			dskKey = anyToString(dsk["dsk_key"])
		}

		printer := Printer{
			ID:         anyToString(pr["station_id"]),
			SN:         stationSN,
			Name:       anyToString(pr["station_name"]),
			Model:      anyToString(pr["station_model"]),
			CreateTime: createTime,
			UpdateTime: updateTime,
			WifiMAC:    anyToString(pr["wifi_mac"]),
			IPAddr:     anyToString(pr["ip_addr"]),
			MQTTKey:    mqttKey,
			APIHosts:   apiHosts,
			P2PHosts:   p2pHosts,
			P2PDUID:    anyToString(pr["p2p_did"]),
			P2PKey:     dskKey,
		}

		cfg.Printers = append(cfg.Printers, printer)
	}

	return cfg, nil
}
