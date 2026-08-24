// Package httpapi implements the HTTP API clients for AnkerMake M5 printers.
//
// This file defines the app, passport, and hub API clients.
// It is the Go equivalent of libflagship/httpapi.py from the Python implementation.
package httpapi

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// APIError represents an HTTP API error.
type APIError struct {
	Message string
	Code    int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("api error: %s (code %d)", e.Message, e.Code)
}

// Region specifies the API region (eu or us).
type Region string

const (
	RegionEU Region = "eu"
	RegionUS Region = "us"
)

// BaseURLs maps regions to their API base URLs.
var BaseURLs = map[Region]string{
	RegionEU: "https://make-app-eu.ankermake.com",
	RegionUS: "https://make-app.ankermake.com",
}

// AnkerHTTPApi is the base HTTP API client.
type AnkerHTTPApi struct {
	BaseURL   string
	AuthToken string
	UserID    string
	Verify    bool
	Scope     string
	client    *http.Client
}

// NewAnkerHTTPApi creates a new base HTTP API client.
func NewAnkerHTTPApi(region Region, authToken string, verify bool, baseURL string) (*AnkerHTTPApi, error) {
	if baseURL == "" {
		var ok bool
		baseURL, ok = BaseURLs[region]
		if !ok {
			return nil, fmt.Errorf("must specify either base_url or region {'eu', 'us'}")
		}
	}

	return &AnkerHTTPApi{
		BaseURL:   baseURL,
		AuthToken: authToken,
		Verify:    verify,
		client:    &http.Client{},
	}, nil
}

// apiResponse is the standard API response wrapper.
type apiResponse struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
}

// get performs a GET request and unwraps the response.
func (a *AnkerHTTPApi) get(url string, headers map[string]string) (json.RawMessage, error) {
	fullURL := a.BaseURL + a.Scope + url

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	a.setCommonHeaders(req, headers)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &APIError{Message: fmt.Sprintf("HTTP %d %s", resp.StatusCode, resp.Status), Code: resp.StatusCode}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var apiResp apiResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("parse API response: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, &APIError{Message: "API error", Code: apiResp.Code}
	}

	return apiResp.Data, nil
}

// post performs a POST request and unwraps the response.
func (a *AnkerHTTPApi) post(url string, headers map[string]string, data any) (json.RawMessage, error) {
	fullURL := a.BaseURL + a.Scope + url

	body, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal request body: %w", err)
	}

	req, err := http.NewRequest("POST", fullURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	a.setCommonHeaders(req, headers)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &APIError{Message: fmt.Sprintf("HTTP %d %s", resp.StatusCode, resp.Status), Code: resp.StatusCode}
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var apiResp apiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("parse API response: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, &APIError{Message: "API error", Code: apiResp.Code}
	}

	return apiResp.Data, nil
}

// ─── App API v1 ──────────────────────────────────────────────────────────────

// AnkerHTTPAppApiV1 is the v1 app API client.
type AnkerHTTPAppApiV1 struct {
	*AnkerHTTPApi
}

// NewAnkerHTTPAppApiV1 creates a new v1 app API client.
func NewAnkerHTTPAppApiV1(region Region, authToken string, verify bool) (*AnkerHTTPAppApiV1, error) {
	base, err := NewAnkerHTTPApi(region, authToken, verify, "")
	if err != nil {
		return nil, err
	}
	base.Scope = "/v1/app"
	return &AnkerHTTPAppApiV1{base}, nil
}

// GetAppVersion queries the latest app version.
func (a *AnkerHTTPAppApiV1) GetAppVersion(appName string, appVersion int, model string) (json.RawMessage, error) {
	return a.post("/ota/get_app_version", nil, map[string]any{
		"app_name":    appName,
		"app_version": appVersion,
		"model":       model,
	})
}

// QueryFdmList queries the FDM printer list. Requires auth token.
// setCommonHeaders sets the Gtoken header (MD5 of user_id) and any extra headers.
func (a *AnkerHTTPApi) setCommonHeaders(req *http.Request, extra map[string]string) {
	if a.UserID != "" {
		h := md5.Sum([]byte(a.UserID))
		req.Header.Set("Gtoken", hex.EncodeToString(h[:]))
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}
}

func (a *AnkerHTTPAppApiV1) QueryFdmList() (json.RawMessage, error) {
	if a.AuthToken == "" {
		return nil, &APIError{Message: "Missing auth token"}
	}
	return a.post("/query_fdm_list", map[string]string{"X-Auth-Token": a.AuthToken}, nil)
}

// EquipmentGetDskKeys gets DSK keys for the given station serial numbers. Requires auth token.
// Bug fix: invalid_dsks is properly initialized (no mutable default argument issue in Go).
func (a *AnkerHTTPAppApiV1) EquipmentGetDskKeys(stationSNs []string, invalidDsks map[string]any) (json.RawMessage, error) {
	if a.AuthToken == "" {
		return nil, &APIError{Message: "Missing auth token"}
	}
	if invalidDsks == nil {
		invalidDsks = map[string]any{}
	}
	return a.post("/equipment/get_dsk_keys", map[string]string{"X-Auth-Token": a.AuthToken}, map[string]any{
		"invalid_dsks": invalidDsks,
		"station_sns":  stationSNs,
	})
}

// ─── Passport API v1 ─────────────────────────────────────────────────────────

// AnkerHTTPPassportApiV1 is the v1 passport API client.
type AnkerHTTPPassportApiV1 struct {
	*AnkerHTTPApi
}

// NewAnkerHTTPPassportApiV1 creates a new v1 passport API client.
func NewAnkerHTTPPassportApiV1(region Region, authToken string, verify bool) (*AnkerHTTPPassportApiV1, error) {
	base, err := NewAnkerHTTPApi(region, authToken, verify, "")
	if err != nil {
		return nil, err
	}
	base.Scope = "/v1/passport"
	return &AnkerHTTPPassportApiV1{base}, nil
}

// Profile gets the user profile. Requires auth token.
func (a *AnkerHTTPPassportApiV1) Profile() (json.RawMessage, error) {
	if a.AuthToken == "" {
		return nil, &APIError{Message: "Missing auth token"}
	}
	return a.get("/profile", map[string]string{"X-Auth-Token": a.AuthToken})
}

// ─── Hub API v1 ──────────────────────────────────────────────────────────────

// AnkerHTTPHubApiV1 is the v1 hub API client.
type AnkerHTTPHubApiV1 struct {
	*AnkerHTTPApi
}

// NewAnkerHTTPHubApiV1 creates a new v1 hub API client.
func NewAnkerHTTPHubApiV1(region Region, authToken string, verify bool) (*AnkerHTTPHubApiV1, error) {
	base, err := NewAnkerHTTPApi(region, authToken, verify, "")
	if err != nil {
		return nil, err
	}
	base.Scope = "/v1/hub"
	return &AnkerHTTPHubApiV1{base}, nil
}

// QueryDeviceInfo queries device info using the v1 API (check_code based).
func (a *AnkerHTTPHubApiV1) QueryDeviceInfo(stationSN, checkCode string) (json.RawMessage, error) {
	return a.post("/query_device_info", nil, map[string]any{
		"station_sn": stationSN,
		"check_code": checkCode,
	})
}

// OTAGetRomVersion queries ROM version info using the v1 API.
func (a *AnkerHTTPHubApiV1) OTAGetRomVersion(printerSN, checkCode, deviceType, currentVersionName string) (json.RawMessage, error) {
	return a.post("/ota/get_rom_version", nil, map[string]any{
		"sn":                   printerSN,
		"check_code":           checkCode,
		"device_type":          deviceType,
		"current_version_name": currentVersionName,
	})
}

// ─── Hub API v2 ──────────────────────────────────────────────────────────────

// AnkerHTTPHubApiV2 is the v2 hub API client.
type AnkerHTTPHubApiV2 struct {
	*AnkerHTTPApi
}

// NewAnkerHTTPHubApiV2 creates a new v2 hub API client.
func NewAnkerHTTPHubApiV2(region Region, authToken string, verify bool) (*AnkerHTTPHubApiV2, error) {
	base, err := NewAnkerHTTPApi(region, authToken, verify, "")
	if err != nil {
		return nil, err
	}
	base.Scope = "/v2/hub"
	return &AnkerHTTPHubApiV2{base}, nil
}

// QueryDeviceInfo queries device info using the v2 API (sec_code based).
func (a *AnkerHTTPHubApiV2) QueryDeviceInfo(stationSN, secCode string, secTs string) (json.RawMessage, error) {
	return a.post("/query_device_info", nil, map[string]any{
		"station_sn": stationSN,
		"sec_code":   secCode,
		"sec_ts":     secTs,
	})
}

// OTAGetRomVersion queries ROM version info using the v2 API.
func (a *AnkerHTTPHubApiV2) OTAGetRomVersion(printerSN, secCode, secTs, deviceType, currentVersionName string) (json.RawMessage, error) {
	return a.post("/ota/get_rom_version", nil, map[string]any{
		"sn":                   printerSN,
		"sec_code":             secCode,
		"sec_ts":               secTs,
		"device_type":          deviceType,
		"current_version_name": currentVersionName,
	})
}

// GetP2PConnectInfo gets P2P connection info.
func (a *AnkerHTTPHubApiV2) GetP2PConnectInfo(printerSN, secCode, secTs string) (json.RawMessage, error) {
	return a.post("/get_p2p_connectinfo", nil, map[string]any{
		"station_sn": printerSN,
		"sec_code":   secCode,
		"sec_ts":     secTs,
	})
}
