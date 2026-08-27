package gcode

import (
	"bufio"
	"encoding/base64"
	"os"
	"strings"
)

// ExtractThumbnail reads a G-code file and extracts the first embedded
// thumbnail image (PrusaSlicer/OrcaSlicer format). Returns the raw image
// bytes (PNG or JPEG) and a data URI suitable for use in an <img> tag.
// Returns empty strings if no thumbnail is found.
func ExtractThumbnail(path string) ([]byte, string) {
	f, err := os.Open(path)
	if err != nil {
		return nil, ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Increase buffer for large thumbnails
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	var b64Lines []string
	var inThumbnail bool
	var imageType string // "png" or "jpeg"

	for scanner.Scan() {
		line := scanner.Text()

		// PrusaSlicer format: ; thumbnail begin 220x124 123456
		// Or: ; thumbnail_QOI begin ...
		if strings.Contains(line, "; thumbnail") && strings.Contains(line, "begin") {
			inThumbnail = true
			b64Lines = nil
			// Determine image type from format hint
			if strings.Contains(line, "QOI") {
				imageType = "qoi"
			} else {
				imageType = "png" // default to PNG
			}
			continue
		}
		if strings.Contains(line, "; thumbnail") && strings.Contains(line, "end") {
			inThumbnail = false
			if len(b64Lines) > 0 {
				break // got the first thumbnail
			}
			continue
		}
		if inThumbnail {
			// Thumbnail data lines start with "; " followed by base64
			trimmed := strings.TrimPrefix(line, "; ")
			trimmed = strings.TrimSpace(trimmed)
			if trimmed != "" && isBase64Char(trimmed[0]) {
				b64Lines = append(b64Lines, trimmed)
			}
		}
	}

	if len(b64Lines) == 0 {
		return nil, ""
	}

	// Decode base64
	combined := strings.Join(b64Lines, "")
	data, err := base64.StdEncoding.DecodeString(combined)
	if err != nil {
		return nil, ""
	}

	// Detect actual image type from magic bytes
	mimeType := "image/png"
	if len(data) >= 3 {
		if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
			mimeType = "image/jpeg"
		} else if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
			mimeType = "image/png"
		}
	}
	_ = imageType // unused, we detect from magic bytes

	dataURI := "data:" + mimeType + ";base64," + combined
	return data, dataURI
}

func isBase64Char(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '+' || c == '/'
}
