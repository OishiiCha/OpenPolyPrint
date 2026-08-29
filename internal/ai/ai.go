package ai

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AnalysisRequest contains all the data for an AI analysis of a print frame.
type AnalysisRequest struct {
	APIKey         string  `json:"apiKey"`
	FramePath      string  `json:"framePath"`    // path to JPEG frame
	FrameDir       string  `json:"frameDir"`     // timelapse frames directory
	FrameNum       int     `json:"frameNum"`     // frame number
	ElapsedSec     float64 `json:"elapsedSec"`   // elapsed seconds in print
	GCodeLine      int     `json:"gcodeLine"`    // current G-code line
	GCodeSnippet   string  `json:"gcodeSnippet"` // surrounding G-code lines
	Layer          int     `json:"layer"`
	X              float64 `json:"x"`
	Y              float64 `json:"y"`
	Z              float64 `json:"z"`
	NozzleTemp     float64 `json:"nozzleTemp"`
	TargetNozzle   float64 `json:"targetNozzle"`
	BedTemp        float64 `json:"bedTemp"`
	TargetBed      float64 `json:"targetBed"`
	PrinterName    string  `json:"printerName"`
	Filename       string  `json:"filename"`
	PromptOverride string  `json:"promptOverride,omitempty"` // user-edited default prompt
	CustomPrompt   string  `json:"customPrompt,omitempty"`   // additional user instructions
}

// AnalysisResponse is the result from Gemini.
type AnalysisResponse struct {
	Analysis    string `json:"analysis"`
	Issues      string `json:"issues,omitempty"`
	Suggestions string `json:"suggestions,omitempty"`
	Raw         string `json:"raw,omitempty"`
}

// Analyze sends a frame + context to Gemini for analysis.
func Analyze(req AnalysisRequest) (*AnalysisResponse, error) {
	if req.APIKey == "" {
		return nil, fmt.Errorf("API key required")
	}

	// Read the frame image
	frameData, err := os.ReadFile(req.FramePath)
	if err != nil {
		return nil, fmt.Errorf("read frame: %w", err)
	}

	// Build the prompt with all context
	prompt := buildFinalPrompt(req)

	// Encode image as base64
	imgBase64 := base64.StdEncoding.EncodeToString(frameData)

	// Build Gemini API request (using gemini-2.5-flash for multimodal with good speed)
	type part struct {
		Text       string `json:"text,omitempty"`
		InlineData struct {
			MimeType string `json:"mimeType"`
			Data     string `json:"data"`
		} `json:"inlineData,omitempty"`
	}
	type content struct {
		Parts []part `json:"parts"`
	}
	type geminiReq struct {
		Contents         []content `json:"contents"`
		GenerationConfig struct {
			Temperature     float64 `json:"temperature"`
			MaxOutputTokens int     `json:"maxOutputTokens"`
		} `json:"generationConfig"`
	}

	body := geminiReq{
		Contents: []content{
			{
				Parts: []part{
					{Text: prompt},
					{
						InlineData: struct {
							MimeType string `json:"mimeType"`
							Data     string `json:"data"`
						}{
							MimeType: "image/jpeg",
							Data:     imgBase64,
						},
					},
				},
			},
		},
		GenerationConfig: struct {
			Temperature     float64 `json:"temperature"`
			MaxOutputTokens int     `json:"maxOutputTokens"`
		}{
			Temperature:     0.4,
			MaxOutputTokens: 2048,
		},
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Use gemini-2.5-flash for multimodal analysis
	url := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=" + req.APIKey

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("gemini API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Parse Gemini response
	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBody, &geminiResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	var text string
	if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
		text = geminiResp.Candidates[0].Content.Parts[0].Text
	}

	return &AnalysisResponse{
		Analysis: text,
		Raw:      string(respBody),
	}, nil
}

// BuildDefaultPrompt generates the default prompt text based on what data is
// available in the request. Sections are included conditionally — e.g. if
// there is no G-code snippet, the G-code section is omitted; if there are no
// temperatures, the temperature section is omitted.
func BuildDefaultPrompt(req AnalysisRequest) string {
	var sb strings.Builder
	sb.WriteString("You are a 3D printing expert analyzing a timelapse frame from a print job. ")
	sb.WriteString("Examine the image and the provided data to identify any issues, defects, or areas for improvement.\n\n")

	// Print Context — only include fields that have values
	hasContext := false
	sb.WriteString("## Print Context\n")
	if req.PrinterName != "" {
		sb.WriteString(fmt.Sprintf("- Printer: %s\n", req.PrinterName))
		hasContext = true
	}
	if req.Filename != "" {
		sb.WriteString(fmt.Sprintf("- File: %s\n", req.Filename))
		hasContext = true
	}
	if req.ElapsedSec > 0 {
		sb.WriteString(fmt.Sprintf("- Elapsed time: %.1f seconds (%.1f minutes)\n", req.ElapsedSec, req.ElapsedSec/60))
		hasContext = true
	}
	if req.Layer > 0 {
		sb.WriteString(fmt.Sprintf("- Layer: %d\n", req.Layer))
		hasContext = true
	}
	if req.X != 0 || req.Y != 0 || req.Z != 0 {
		sb.WriteString(fmt.Sprintf("- Nozzle position: X=%.2f Y=%.2f Z=%.2f\n", req.X, req.Y, req.Z))
		hasContext = true
	}
	if req.FrameNum > 0 {
		sb.WriteString(fmt.Sprintf("- Frame number: %d\n", req.FrameNum))
		hasContext = true
	}
	if !hasContext {
		sb.WriteString("(no metadata available)\n")
	}
	sb.WriteString("\n")

	// Temperature section — only if we have temp data
	hasTemps := req.NozzleTemp > 0 || req.BedTemp > 0 || req.TargetNozzle > 0 || req.TargetBed > 0
	if hasTemps {
		sb.WriteString("## Temperature Data\n")
		if req.NozzleTemp > 0 || req.TargetNozzle > 0 {
			sb.WriteString(fmt.Sprintf("- Nozzle temp: %.1f°C (target: %.1f°C)\n", req.NozzleTemp, req.TargetNozzle))
		}
		if req.BedTemp > 0 || req.TargetBed > 0 {
			sb.WriteString(fmt.Sprintf("- Bed temp: %.1f°C (target: %.1f°C)\n", req.BedTemp, req.TargetBed))
		}
		sb.WriteString("\n")
	}

	// G-code section — only if we have a snippet
	if req.GCodeSnippet != "" {
		sb.WriteString("## Current G-code (around current position)\n")
		sb.WriteString("```\n")
		sb.WriteString(req.GCodeSnippet)
		sb.WriteString("\n```\n\n")
	}

	// Analysis Instructions — adapt based on what data is available
	sb.WriteString("## Analysis Instructions\n")
	sb.WriteString("1. Describe what you see in the image (print quality, layer adhesion, any visible defects)\n")
	sb.WriteString("2. Identify any issues: stringing, warping, layer shifting, under-extrusion, over-extrusion, adhesion problems, blobbing, etc.\n")
	sb.WriteString("3. If issues are found, suggest specific fixes (temperature adjustments, retraction settings, speed changes, etc.)\n")
	sb.WriteString("4. If the print looks good, confirm that and note any minor improvements\n")
	if hasTemps {
		sb.WriteString("5. Consider the temperature data — are the actual temps matching targets? Any concerning trends?\n")
	}
	if req.GCodeSnippet != "" {
		sb.WriteString("6. Review the current G-code — does the commanded move match what you see in the image?\n")
	}
	sb.WriteString("\n")

	sb.WriteString("Format your response as:\n")
	sb.WriteString("**Observations:** [what you see]\n")
	sb.WriteString("**Issues:** [any problems found, or 'None detected']\n")
	sb.WriteString("**Suggestions:** [specific actionable recommendations]\n")

	return sb.String()
}

// buildFinalPrompt assembles the prompt sent to Gemini:
//   - If PromptOverride is set, use it instead of the generated default.
//   - If CustomPrompt is set, append it as additional user instructions.
func buildFinalPrompt(req AnalysisRequest) string {
	var prompt string
	if req.PromptOverride != "" {
		prompt = req.PromptOverride
	} else {
		prompt = BuildDefaultPrompt(req)
	}
	if req.CustomPrompt != "" {
		prompt += "\n\n## Additional User Instructions\n" + req.CustomPrompt + "\n"
	}
	return prompt
}

// FindFrameForTime returns the frame file path closest to the given elapsed time.
// Frames are named frame_000001.jpg, frame_000002.jpg, etc.
// The frame number corresponds to the timelapse interval, so frame N at interval I
// corresponds to elapsed time N*I seconds.
func FindFrameForTime(framesDir string, elapsedSec float64, intervalSec float64) (string, int, error) {
	if intervalSec <= 0 {
		intervalSec = 1.0
	}
	frameNum := int(elapsedSec / intervalSec)
	if frameNum < 1 {
		frameNum = 1
	}
	framePath := filepath.Join(framesDir, fmt.Sprintf("frame_%06d.jpg", frameNum))
	if _, err := os.Stat(framePath); err != nil {
		// Try to find the closest frame
		entries, err2 := os.ReadDir(framesDir)
		if err2 != nil {
			return "", 0, fmt.Errorf("no frame found: %w", err)
		}
		var frames []string
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".jpg") {
				frames = append(frames, e.Name())
			}
		}
		if len(frames) == 0 {
			return "", 0, fmt.Errorf("no frames in directory")
		}
		// Use the closest frame
		if frameNum > len(frames) {
			frameNum = len(frames)
		}
		framePath = filepath.Join(framesDir, frames[frameNum-1])
	}
	return framePath, frameNum, nil
}
