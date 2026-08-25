package gcode

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"strings"
)

// Segment represents a single G-code movement with an estimated timestamp.
type Segment struct {
	LineNum     int     `json:"lineNum"`
	Layer       int     `json:"layer"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	Z           float64 `json:"z"`
	E           float64 `json:"e"`
	Extruding   bool    `json:"extruding"`
	Feeedrate   float64 `json:"feedrate"`    // mm/min
	Distance    float64 `json:"distance"`    // mm
	Duration    float64 `json:"duration"`    // seconds for this move
	ElapsedTime float64 `json:"elapsedTime"` // cumulative seconds from start
	GCode       string  `json:"gcode"`
}

// ParseTimeline parses G-code and returns timestamped segments with estimated
// print time. The feedrate (F) values are in mm/min as per G-code standard.
// Travel moves and extrusion moves are both included.
func ParseTimeline(r io.Reader) ([]Segment, error) {
	scanner := bufio.NewScanner(r)
	// Allow large lines
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	var segments []Segment
	x, y, z, e := 0.0, 0.0, 0.0, 0.0
	feedrate := 1500.0 // default 1500 mm/min (25 mm/s)
	relative := false
	eRelative := false
	layer := 0
	prevZ := 0.0
	elapsed := 0.0
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		raw := scanner.Text()
		ln := strings.TrimSpace(raw)
		// Skip comments and empty lines
		if ln == "" || strings.HasPrefix(ln, ";") {
			// Check for layer indicator in comments (Slic3r/PrusaSlicer style)
			if strings.Contains(ln, ";LAYER:") {
				if _, err := fmt.Sscanf(ln, ";LAYER:%d", &layer); err == nil {
					// layer parsed
				}
			}
			continue
		}

		// Strip inline comments
		if idx := strings.Index(ln, ";"); idx >= 0 {
			ln = strings.TrimSpace(ln[:idx])
		}

		tokens := strings.Fields(ln)
		if len(tokens) == 0 {
			continue
		}
		cmd := tokens[0]

		switch cmd {
		case "G90":
			relative = false
			continue
		case "G91":
			relative = true
			continue
		case "G92":
			for _, t := range tokens[1:] {
				if len(t) < 2 {
					continue
				}
				v := parseFloatSafe(t[1:])
				switch t[0] {
				case 'X':
					x = v
				case 'Y':
					y = v
				case 'Z':
					z = v
				case 'E':
					e = v
				}
			}
			continue
		case "G28":
			// Homing — reset positions
			homeAll := len(tokens) == 1
			for _, t := range tokens[1:] {
				if len(t) >= 1 {
					switch t[0] {
					case 'X':
						x = 0
					case 'Y':
						y = 0
					case 'Z':
						z = 0
					}
				}
			}
			if homeAll {
				x, y, z = 0, 0, 0
			}
			// Estimate homing time as 5 seconds
			elapsed += 5.0
			continue
		case "M82":
			eRelative = false
			continue
		case "M83":
			eRelative = true
			continue
		case "G0", "G1":
			// Movement command
		default:
			continue
		}

		// Parse G0/G1
		nx, ny, nz, ne := x, y, z, e
		hasE := false
		for _, t := range tokens[1:] {
			if len(t) < 2 {
				continue
			}
			v := parseFloatSafe(t[1:])
			switch t[0] {
			case 'X':
				if relative {
					nx = x + v
				} else {
					nx = v
				}
			case 'Y':
				if relative {
					ny = y + v
				} else {
					ny = v
				}
			case 'Z':
				if relative {
					nz = z + v
				} else {
					nz = v
				}
			case 'E':
				hasE = true
				if eRelative {
					ne = e + v
				} else {
					ne = v
				}
			case 'F':
				feedrate = v
			}
		}

		// Calculate distance
		dx := nx - x
		dy := ny - y
		dz := nz - z
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

		// Calculate duration: feedrate is mm/min, so mm/s = feedrate/60
		speed := feedrate / 60.0
		if speed < 0.1 {
			speed = 25.0 // fallback 25mm/s
		}
		duration := dist / speed

		// Determine if extruding
		extruding := hasE && ne > e

		// Detect layer change by Z movement
		if nz != prevZ && nz > prevZ && extruding {
			layer++
			prevZ = nz
		}

		seg := Segment{
			LineNum:     lineNum,
			Layer:       layer,
			X:           nx,
			Y:           ny,
			Z:           nz,
			E:           ne,
			Extruding:   extruding,
			Feeedrate:   feedrate,
			Distance:    dist,
			Duration:    duration,
			ElapsedTime: elapsed,
			GCode:       ln,
		}
		segments = append(segments, seg)

		elapsed += duration
		x, y, z, e = nx, ny, nz, ne
	}

	if err := scanner.Err(); err != nil {
		return segments, err
	}
	return segments, nil
}

// SegmentAtTime returns the segment closest to the given elapsed time.
func SegmentAtTime(segments []Segment, elapsed float64) *Segment {
	if len(segments) == 0 {
		return nil
	}
	// Binary search for the segment at or just before the elapsed time
	lo, hi := 0, len(segments)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if segments[mid].ElapsedTime <= elapsed {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return &segments[lo]
}

// SegmentAtProgress returns the segment at the given progress (0-1).
func SegmentAtProgress(segments []Segment, progress float64) *Segment {
	if len(segments) == 0 {
		return nil
	}
	totalTime := segments[len(segments)-1].ElapsedTime
	if totalTime <= 0 {
		return &segments[0]
	}
	return SegmentAtTime(segments, progress*totalTime)
}

func parseFloatSafe(s string) float64 {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	if err != nil {
		return 0
	}
	return f
}
