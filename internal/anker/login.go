package anker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ankermgmt/ankermake-m5-protocol-go/flagship/config"
	"github.com/ankermgmt/ankermake-m5-protocol-go/flagship/crypto"
)

// LoginResponse is the public result of an AnkerMake login or import operation.
type LoginResponse struct {
	Success          bool            `json:"success"`
	Message          string          `json:"message,omitempty"`
	Code             int             `json:"code,omitempty"`
	CaptchaID        string          `json:"captcha_id,omitempty"`
	CaptchaImg       string          `json:"captcha_img,omitempty"`
	VerificationData json.RawMessage `json:"verification_data,omitempty"`
}

// Login performs an email/password login to the AnkerMake cloud.
// If captchaID/captchaAnswer are provided, they are passed through to the API.
// If verificationData/verificationCode are provided, the server-supplied context
// (e.g. verify_id, code_id) is merged back into the login body.
func Login(email, password, region, captchaID, captchaAnswer, verificationCode string, verificationData map[string]any, cfgMgr *config.ConfigManager) (LoginResponse, *config.Config, error) {
	if email == "" || password == "" {
		return LoginResponse{Success: false, Message: "Email and password are required"}, nil, nil
	}

	if region == "" {
		region = "eu"
	}

	encResult, err := crypto.ECDHEncryptLoginPassword([]byte(password))
	if err != nil {
		return LoginResponse{Success: false, Message: fmt.Sprintf("Password encryption failed: %v", err)}, nil, err
	}

	loginBody := map[string]any{
		"client_secret_info": map[string]string{"public_key": encResult.PublicKey},
		"email":              email,
		"password":           encResult.Ciphertext,
	}
	if captchaID != "" {
		loginBody["captcha_id"] = captchaID
	}
	if captchaAnswer != "" {
		loginBody["answer"] = captchaAnswer
	}
	for k, v := range verificationData {
		if k == "captcha_img" || k == "item" {
			continue
		}
		loginBody[k] = v
	}
	if verificationCode != "" {
		loginBody["verification_code"] = verificationCode
	}

	var baseURL string
	if region == "us" {
		baseURL = "https://make-app.ankermake.com"
	} else {
		baseURL = "https://make-app-eu.ankermake.com"
	}
	loginURL := baseURL + "/v2/passport/login"

	bodyBytes, _ := json.Marshal(loginBody)
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("POST", loginURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return LoginResponse{Success: false, Message: fmt.Sprintf("Create request: %v", err)}, nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, */*;q=0.8")
	req.Header.Set("User-Agent", "python-requests/2.31.0")
	req.Header.Set("App_name", "anker_make")
	req.Header.Set("App_version", "")
	req.Header.Set("Model_type", "PC")
	req.Header.Set("Os_type", "windows")
	req.Header.Set("Os_version", "10sp1")

	resp, err := client.Do(req)
	if err != nil {
		return LoginResponse{Success: false, Message: fmt.Sprintf("Login request failed: %v", err)}, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return LoginResponse{Success: false, Message: fmt.Sprintf("Read response: %v", err)}, nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return LoginResponse{Success: false, Message: fmt.Sprintf("Login API returned HTTP %d %s. Body: %s", resp.StatusCode, resp.Status, string(respBody))}, nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	if len(respBody) == 0 {
		return LoginResponse{Success: false, Message: "Login API returned empty response"}, nil, fmt.Errorf("empty response")
	}

	var apiResp struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return LoginResponse{Success: false, Message: fmt.Sprintf("Parse response: %v. Body: %s", err, string(respBody))}, nil, err
	}

	if apiResp.Code == 100032 {
		var captchaData struct {
			CaptchaID  string `json:"captcha_id"`
			CaptchaImg string `json:"captcha_img"`
			Item       string `json:"item"`
		}
		_ = json.Unmarshal(apiResp.Data, &captchaData)
		img := captchaData.CaptchaImg
		if img == "" {
			img = captchaData.Item
		}
		return LoginResponse{
			Success:    false,
			Message:    "CAPTCHA required",
			Code:       100032,
			CaptchaID:  captchaData.CaptchaID,
			CaptchaImg: img,
		}, nil, nil
	}

	if apiResp.Code != 0 {
		// Return the raw data so the frontend can ask for an email/phone
		// verification code, 2FA code, or other follow-up prompt.
		return LoginResponse{
			Success:          false,
			Message:          fmt.Sprintf("Login failed (code %d): %s", apiResp.Code, apiResp.Msg),
			Code:             apiResp.Code,
			VerificationData: apiResp.Data,
		}, nil, fmt.Errorf("code %d", apiResp.Code)
	}

	var loginData struct {
		UserID    string `json:"user_id"`
		AuthToken string `json:"auth_token"`
		AbCode    string `json:"ab_code"`
	}
	if err := json.Unmarshal(apiResp.Data, &loginData); err != nil {
		return LoginResponse{Success: false, Message: fmt.Sprintf("Parse login data: %v", err)}, nil, err
	}

	determinedRegion := crypto.GuessRegion(loginData.AbCode)
	cfg, err := config.LoadFromAPIWithUserID(loginData.AuthToken, determinedRegion, loginData.UserID, false)
	if err != nil {
		return LoginResponse{Success: false, Message: fmt.Sprintf("Login succeeded but config fetch failed: %v", err)}, nil, err
	}
	log.Printf("login: loaded %d printer(s) for user %s", len(cfg.Printers), loginData.UserID)

	if err := cfgMgr.Save("default", cfg); err != nil {
		return LoginResponse{Success: false, Message: fmt.Sprintf("Save config: %v", err)}, nil, err
	}

	return LoginResponse{Success: true, Message: "Login successful. Config imported."}, cfg, nil
}

// ImportLoginJSON imports a previously extracted login.json file.
func ImportLoginJSON(loginData []byte, cfgMgr *config.ConfigManager) (LoginResponse, *config.Config, error) {
	cache, err := crypto.LoadLoginCacheDefault(loginData)
	if err != nil {
		return LoginResponse{Success: false, Message: fmt.Sprintf("Import failed: %v", err)}, nil, err
	}

	data, _ := cache["data"].(map[string]any)
	authToken, _ := data["auth_token"].(string)
	abCode, _ := data["ab_code"].(string)
	userID, _ := data["user_id"].(string)

	region := crypto.GuessRegion(abCode)

	cfg, err := config.LoadFromAPIWithUserID(authToken, region, userID, false)
	if err != nil {
		return LoginResponse{Success: false, Message: fmt.Sprintf("Import failed: %v. Auth token might be expired: make sure Ankermake Slicer can connect, then try again", err)}, nil, err
	}
	log.Printf("import: loaded %d printer(s) for user %s", len(cfg.Printers), userID)

	if err := cfgMgr.Save("default", cfg); err != nil {
		return LoginResponse{Success: false, Message: fmt.Sprintf("Save config: %v", err)}, nil, err
	}

	// Store the uploaded login.json for later reloads.
	if err := os.MkdirAll(cfgMgr.ConfigDir(), 0o755); err != nil {
		return LoginResponse{Success: false, Message: fmt.Sprintf("Create config dir: %v", err)}, nil, err
	}
	loginPath := filepath.Join(cfgMgr.ConfigDir(), "login.json")
	if err := os.WriteFile(loginPath, loginData, 0o600); err != nil {
		return LoginResponse{Success: false, Message: fmt.Sprintf("Save login.json: %v", err)}, nil, err
	}

	return LoginResponse{Success: true, Message: "Config imported from login.json."}, cfg, nil
}

// expandWindowsEnv expands %VAR% style environment variables on Windows.
func expandWindowsEnv(s string) string {
	for {
		start := strings.Index(s, "%")
		if start < 0 {
			break
		}
		end := strings.Index(s[start+1:], "%")
		if end < 0 {
			break
		}
		end += start + 1
		varName := s[start+1 : end]
		val := os.Getenv(varName)
		if val != "" {
			s = s[:start] + val + s[end+1:]
		} else {
			s = s[:start] + s[end+1:]
		}
	}
	return s
}

func expandLoginPath(path string) string {
	path = expandWindowsEnv(path)
	path = os.ExpandEnv(path)
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		path = filepath.Join(home, path[1:])
	}
	return path
}

// FindLoginJSON returns the first likely login.json path that exists, or "".
// This mirrors the logic in ankerctl's cmd/ankerctl/config.go defaultLoginJSONPath.
func FindLoginJSON() string {
	var candidates []string
	switch runtime.GOOS {
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData != "" {
			candidates = []string{
				localAppData + `\Ankermake\AnkerMake_64bit_fp\login.json`,
				localAppData + `\Ankermake\login.json`,
			}
		}
	case "darwin":
		home, err := os.UserHomeDir()
		if err == nil {
			candidates = []string{
				home + "/Library/Application Support/AnkerMake/AnkerMake_64bit_fp/login.json",
			}
		}
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// AutoImport tries to detect and import a slicer login.json.
func AutoImport(cfgMgr *config.ConfigManager) (LoginResponse, *config.Config, error) {
	path := FindLoginJSON()
	if path == "" {
		return LoginResponse{Success: false, Message: "No login.json found. Install and log in to AnkerMake/eufyMake Studio, or upload the file manually."}, nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return LoginResponse{Success: false, Message: fmt.Sprintf("Read %s: %v", path, err)}, nil, err
	}

	return ImportLoginJSON(data, cfgMgr)
}
