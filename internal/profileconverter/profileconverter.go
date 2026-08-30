package profileconverter

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Format identifies the source or target slicer format.
type Format string

const (
	FormatCura        Format = "cura"        // Cura / AnkerMake Slicer (.inst.cfg + .def.json)
	FormatPrusaSlicer Format = "prusaslicer" // PrusaSlicer / eufyMake Studio (.ini)
)

// ConversionResult holds the converted profile content and metadata.
type ConversionResult struct {
	Content  string   `json:"content"`
	Filename string   `json:"filename"`
	Format   Format   `json:"format"`   // target format
	Warnings []string `json:"warnings"` // settings that couldn't be mapped
	Unmapped []string `json:"unmapped"` // source keys with no mapping
	Sections int      `json:"sections"` // number of sections in output
	SavedID  string   `json:"savedId,omitempty"`
}

// Convert detects the source format and converts to the target format.
func Convert(content string, filename string, target Format) (*ConversionResult, error) {
	source := DetectFormat(content, filename)
	if source == target {
		return &ConversionResult{
			Content:  content,
			Filename: filename,
			Format:   target,
			Warnings: []string{"Source and target format are the same, no conversion needed"},
		}, nil
	}

	switch source {
	case FormatCura:
		if target == FormatPrusaSlicer {
			return curaToPrusaSlicer(content, filename)
		}
	case FormatPrusaSlicer:
		if target == FormatCura {
			return prusaSlicerToCura(content, filename)
		}
	}
	return nil, fmt.Errorf("unsupported conversion: %s → %s", source, target)
}

// DetectFormat tries to determine the profile format from content and filename.
func DetectFormat(content, filename string) Format {
	// JSON content → Cura .def.json
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "{") {
		return FormatCura
	}
	// .inst.cfg files have [general] and [values] sections
	if strings.Contains(content, "[general]") && strings.Contains(content, "[values]") {
		return FormatCura
	}
	// PrusaSlicer INI has sections like [print:...], [filament:...], [printer:...]
	if strings.Contains(content, "[print:") || strings.Contains(content, "[filament:") || strings.Contains(content, "[printer:") {
		return FormatPrusaSlicer
	}
	// Check filename extension
	lower := strings.ToLower(filename)
	if strings.HasSuffix(lower, ".def.json") || strings.HasSuffix(lower, ".inst.cfg") {
		return FormatCura
	}
	if strings.HasSuffix(lower, ".ini") {
		return FormatPrusaSlicer
	}
	// Default: treat as PrusaSlicer INI
	return FormatPrusaSlicer
}

// ─── Cura → PrusaSlicer ──────────────────────────────────────────────────────

// curaSettingMap maps Cura setting names to PrusaSlicer equivalents.
// The value is (prusaslicer_key, section) where section is "print", "filament", or "printer".
// A transform function can be applied to convert the value.
type curaMapping struct {
	psKey     string
	section   string // "print", "filament", "printer"
	transform func(string) string
}

var curaToPSMap = map[string]curaMapping{
	// Layer
	"layer_height":   {"layer_height", "print", nil},
	"layer_height_0": {"first_layer_height", "print", nil},
	"line_width":     {"nozzle_diameter", "printer", divBy100}, // approximate

	// Speed
	"speed_print":     {"infill_speed", "print", nil},
	"speed_infill":    {"infill_speed", "print", nil},
	"speed_wall":      {"perimeter_speed", "print", nil},
	"speed_wall_0":    {"external_perimeter_speed", "print", nil},
	"speed_wall_x":    {"external_perimeter_speed", "print", nil},
	"speed_topbottom": {"solid_infill_speed", "print", nil},
	"speed_travel":    {"travel_speed", "print", nil},
	"speed_layer_0":   {"first_layer_speed", "print", nil},
	"speed_support":   {"support_material_speed", "print", nil},

	// Temperature
	"material_print_temperature":         {"temperature", "filament", nil},
	"material_print_temperature_layer_0": {"first_layer_temperature", "filament", nil},
	"material_bed_temperature":           {"bed_temperature", "filament", nil},
	"material_bed_temperature_layer_0":   {"first_layer_bed_temperature", "filament", nil},
	"material_initial_print_temperature": {"nozzle_temperature_initial_layer", "filament", nil},
	"material_final_print_temperature":   {"temperature", "filament", nil},
	"material_standby_temperature":       {"standby_temperature", "filament", nil},

	// Material
	"material_diameter":     {"filament_diameter", "filament", nil},
	"material_flow":         {"extrusion_multiplier", "filament", divBy100},
	"material_flow_layer_0": {"first_layer_extrusion_width", "print", nil},
	"material_type":         {"filament_type", "filament", upperCase},

	// Retraction
	"retraction_amount":      {"retract_length", "print", nil},
	"retraction_speed":       {"retract_speed", "print", nil},
	"retraction_hop":         {"retract_lift", "print", nil},
	"retraction_hop_enabled": {"retract_lift", "print", boolToVal},
	"retraction_count_max":   {"retract_restart_extra", "print", nil},
	"retraction_min_travel":  {"retract_before_travel", "print", nil},

	// Walls / Perimeters
	"wall_line_count":  {"perimeters", "print", nil},
	"wall_thickness":   {"perimeters", "print", nil}, // approximate
	"wall_0_wipe_dist": {"wipe", "print", nil},

	// Top/Bottom
	"top_layers":           {"top_solid_layers", "print", nil},
	"bottom_layers":        {"bottom_solid_layers", "print", nil},
	"top_bottom_thickness": {"top_solid_thickness", "print", nil},

	// Infill
	"infill_sparse_density": {"fill_density", "print", nil}, // both use %
	"infill_pattern":        {"fill_pattern", "print", curaInfillToPS},
	"infill_overlap":        {"infill_overlap", "print", nil},

	// Skirt/Brim
	"skirt_line_count": {"skirts", "print", nil},
	"skirt_gap":        {"skirt_distance", "print", nil},
	"skirt_brim_speed": {"skirt_speed", "print", nil},
	"brim_line_count":  {"brim_width", "print", nil}, // approximate (lines → mm)
	"brim_width":       {"brim_width", "print", nil},

	// Support
	"support_enable":            {"support_material", "print", boolToVal},
	"support_structure":         {"support_material_style", "print", curaSupportToPS},
	"support_angle":             {"support_material_threshold", "print", nil},
	"support_xy_distance":       {"support_material_xy_spacing", "print", nil},
	"support_z_distance":        {"support_material_contact_distance", "print", nil},
	"support_infill_rate":       {"support_material_density", "print", nil},
	"support_interface_enable":  {"support_material_interface_layers", "print", nil},
	"support_interface_density": {"support_material_interface_density", "print", nil},
	"support_interface_pattern": {"support_material_interface_pattern", "print", nil},
	"support_roof_enable":       {"support_material_interface_layers", "print", nil},
	"support_brim_width":        {"support_material_brim_width", "print", nil},

	// Adhesion
	"adhesion_type": {"_adhesion_type", "print", nil}, // handled specially

	// Cooling
	"cool_min_layer_time": {"slowdown_below_layer_time", "print", nil},
	"cool_min_speed":      {"min_print_speed", "print", nil},
	"cool_fan_enabled":    {"fan_always_on", "print", boolToVal},
	"cool_fan_speed":      {"fan_min_speed", "print", nil},
	"cool_fan_speed_max":  {"fan_max_speed", "print", nil},
	"fan_max_speed":       {"fan_max_speed", "print", nil},

	// Acceleration
	"acceleration_print":   {"acceleration", "print", nil},
	"acceleration_travel":  {"travel_acceleration", "print", nil},
	"acceleration_enabled": {"_acceleration_enabled", "print", boolToVal},

	// Machine
	"machine_width":       {"_bed_width", "printer", nil},
	"machine_depth":       {"_bed_depth", "printer", nil},
	"machine_height":      {"max_print_height", "printer", nil},
	"machine_nozzle_size": {"nozzle_diameter", "printer", nil},
	"machine_name":        {"_machine_name", "printer", nil},
	"machine_heated_bed":  {"_heated_bed", "printer", boolToVal},

	// Misc
	"hole_xy_offset":      {"xy_hole_compensation", "print", nil},
	"small_hole_max_size": {"xy_size_compensation", "print", nil},
	"skin_monotonic":      {"monotonic_solid_infill", "print", boolToVal},
	"zig_zaggify_infill":  {"infill_direction", "print", nil},
}

func curaToPrusaSlicer(content string, filename string) (*ConversionResult, error) {
	warnings := []string{}
	unmapped := []string{}

	// Parse Cura content — could be .inst.cfg (INI) or .def.json (JSON)
	var curaValues map[string]string
	var curaMeta map[string]string

	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "{") {
		// JSON .def.json file
		curaValues, curaMeta = parseCuraJSON(content)
	} else {
		// INI .inst.cfg file
		curaValues, curaMeta = parseCuraCFG(content)
	}

	// Build PrusaSlicer sections
	printSettings := map[string]string{}
	filamentSettings := map[string]string{}
	printerSettings := map[string]string{}

	var adhesionType string
	var accelerationEnabled bool
	var bedWidth, bedDepth string
	var machineName string
	var heatedBed bool

	for curaKey, curaVal := range curaValues {
		mapping, ok := curaToPSMap[curaKey]
		if !ok {
			// Check if it's a known unmappable key
			if isCuraMetaKey(curaKey) {
				continue
			}
			unmapped = append(unmapped, curaKey)
			continue
		}

		val := curaVal
		if mapping.transform != nil {
			val = mapping.transform(val)
		}

		// Handle special keys
		switch mapping.psKey {
		case "_adhesion_type":
			adhesionType = curaVal
			continue
		case "_acceleration_enabled":
			accelerationEnabled = curaVal == "true" || curaVal == "1"
			continue
		case "_bed_width":
			bedWidth = curaVal
			continue
		case "_bed_depth":
			bedDepth = curaVal
			continue
		case "_machine_name":
			machineName = curaVal
			continue
		case "_heated_bed":
			heatedBed = curaVal == "true" || curaVal == "1"
			continue
		}

		switch mapping.section {
		case "print":
			printSettings[mapping.psKey] = val
		case "filament":
			filamentSettings[mapping.psKey] = val
		case "printer":
			printerSettings[mapping.psKey] = val
		}
	}

	// Handle adhesion type
	if adhesionType != "" {
		switch adhesionType {
		case "skirt":
			if _, ok := printSettings["skirts"]; !ok {
				printSettings["skirts"] = "1"
			}
		case "brim":
			printSettings["brim_width"] = "3"
		case "raft":
			printSettings["raft_layers"] = "1"
			warnings = append(warnings, "Raft adhesion converted to basic raft_layers; PrusaSlicer raft settings differ from Cura")
		}
	}

	// Handle bed shape
	if bedWidth != "" && bedDepth != "" {
		printerSettings["bed_shape"] = fmt.Sprintf("0x0,%sx0,%sx%s,0x%s", bedWidth, bedWidth, bedDepth, bedDepth)
	}

	// Handle acceleration
	if accelerationEnabled {
		// PrusaSlicer doesn't have a single enable flag; values being set implies enabled
	} else {
		delete(printSettings, "acceleration")
		delete(printSettings, "travel_acceleration")
	}

	// Determine profile name
	profileName := curaMeta["name"]
	if profileName == "" {
		profileName = strings.TrimSuffix(filename, filepath(filename))
	}
	if machineName != "" {
		profileName = profileName + " @" + machineName
	}

	// Build INI output
	var sb strings.Builder
	sb.WriteString("# Generated by OpenPolyPrint profile converter\n")
	sb.WriteString("# Source: Cura/AnkerMake Slicer format\n")
	sb.WriteString("# Target: PrusaSlicer/eufyMake Studio format\n\n")

	// Print section
	if len(printSettings) > 0 {
		sb.WriteString(fmt.Sprintf("[print:%s]\n", profileName))
		writeSettings(&sb, printSettings)
		sb.WriteString("\n")
	}

	// Filament section
	if len(filamentSettings) > 0 {
		sb.WriteString(fmt.Sprintf("[filament:Generic @%s]\n", machineName))
		writeSettings(&sb, filamentSettings)
		sb.WriteString("\n")
	}

	// Printer section
	if len(printerSettings) > 0 || machineName != "" {
		printerName := machineName
		if printerName == "" {
			printerName = "AnkerMake M5"
		}
		sb.WriteString(fmt.Sprintf("[printer:%s]\n", printerName))
		if heatedBed {
			printerSettings["heated_bed"] = "1"
		}
		writeSettings(&sb, printerSettings)
		sb.WriteString("\n")
	}

	// Sort unmapped for stable output
	sort.Strings(unmapped)
	if len(unmapped) > 0 {
		warnings = append(warnings, fmt.Sprintf("%d settings could not be mapped: %s", len(unmapped), strings.Join(unmapped, ", ")))
	}

	result := &ConversionResult{
		Content:  sb.String(),
		Filename: strings.TrimSuffix(filename, filepath(filename)) + "_prusaslicer.ini",
		Format:   FormatPrusaSlicer,
		Warnings: warnings,
		Unmapped: unmapped,
		Sections: countSections(sb.String()),
	}
	return result, nil
}

// ─── PrusaSlicer → Cura ──────────────────────────────────────────────────────

var psToCuraMap = map[string]curaMapping{
	// Layer
	"layer_height":       {"layer_height", "print", nil},
	"first_layer_height": {"layer_height_0", "print", nil},

	// Speed
	"infill_speed":             {"speed_infill", "print", nil},
	"perimeter_speed":          {"speed_wall", "print", nil},
	"external_perimeter_speed": {"speed_wall_x", "print", nil},
	"solid_infill_speed":       {"speed_topbottom", "print", nil},
	"travel_speed":             {"speed_travel", "print", nil},
	"first_layer_speed":        {"speed_layer_0", "print", nil},
	"support_material_speed":   {"speed_support", "print", nil},

	// Temperature
	"temperature":                 {"material_print_temperature", "filament", nil},
	"first_layer_temperature":     {"material_print_temperature_layer_0", "filament", nil},
	"bed_temperature":             {"material_bed_temperature", "filament", nil},
	"first_layer_bed_temperature": {"material_bed_temperature_layer_0", "filament", nil},

	// Material
	"filament_diameter":    {"material_diameter", "filament", nil},
	"extrusion_multiplier": {"material_flow", "filament", mulBy100},
	"filament_type":        {"material_type", "filament", lowerCase},

	// Retraction
	"retract_length":        {"retraction_amount", "print", nil},
	"retract_speed":         {"retraction_speed", "print", nil},
	"retract_lift":          {"retraction_hop", "print", nil},
	"retract_before_travel": {"retraction_min_travel", "print", nil},

	// Walls
	"perimeters": {"wall_line_count", "print", nil},

	// Top/Bottom
	"top_solid_layers":    {"top_layers", "print", nil},
	"bottom_solid_layers": {"bottom_layers", "print", nil},
	"top_solid_thickness": {"top_bottom_thickness", "print", nil},

	// Infill
	"fill_density": {"infill_sparse_density", "print", nil},
	"fill_pattern": {"infill_pattern", "print", psInfillToCura},

	// Skirt/Brim
	"skirts":         {"skirt_line_count", "print", nil},
	"skirt_distance": {"skirt_gap", "print", nil},
	"brim_width":     {"brim_width", "print", nil},

	// Support
	"support_material":                  {"support_enable", "print", valToBool},
	"support_material_style":            {"support_structure", "print", psSupportToCura},
	"support_material_threshold":        {"support_angle", "print", nil},
	"support_material_xy_spacing":       {"support_xy_distance", "print", nil},
	"support_material_contact_distance": {"support_z_distance", "print", nil},
	"support_material_density":          {"support_infill_rate", "print", nil},

	// Cooling
	"slowdown_below_layer_time": {"cool_min_layer_time", "print", nil},
	"min_print_speed":           {"cool_min_speed", "print", nil},
	"fan_min_speed":             {"cool_fan_speed", "print", nil},
	"fan_max_speed":             {"cool_fan_speed_max", "print", nil},

	// Acceleration
	"acceleration":        {"acceleration_print", "print", nil},
	"travel_acceleration": {"acceleration_travel", "print", nil},

	// Machine
	"max_print_height": {"machine_height", "printer", nil},
	"nozzle_diameter":  {"machine_nozzle_size", "printer", nil},
}

func prusaSlicerToCura(content string, filename string) (*ConversionResult, error) {
	warnings := []string{}
	unmapped := []string{}

	// Parse PrusaSlicer INI
	sections := parsePrusaSlicerINI(content)

	// Collect all settings from all sections
	allSettings := map[string]string{}
	var printSectionName, filamentSectionName, printerSectionName string
	_ = filamentSectionName
	_ = printerSectionName

	for _, sec := range sections {
		switch {
		case strings.HasPrefix(sec.name, "print:"):
			printSectionName = sec.name
			for k, v := range sec.keys {
				allSettings[k] = v
			}
		case strings.HasPrefix(sec.name, "filament:"):
			filamentSectionName = sec.name
			for k, v := range sec.keys {
				allSettings[k] = v
			}
		case strings.HasPrefix(sec.name, "printer:"):
			printerSectionName = sec.name
			for k, v := range sec.keys {
				allSettings[k] = v
			}
		}
	}

	// Build Cura values
	curaValues := map[string]string{}

	for psKey, psVal := range allSettings {
		mapping, ok := psToCuraMap[psKey]
		if !ok {
			if isPSMetaKey(psKey) {
				continue
			}
			unmapped = append(unmapped, psKey)
			continue
		}

		val := psVal
		if mapping.transform != nil {
			val = mapping.transform(val)
		}
		curaValues[mapping.psKey] = val
	}

	// Parse bed_shape to get width/depth
	if bedShape, ok := allSettings["bed_shape"]; ok {
		w, d := parseBedShape(bedShape)
		if w != "" && d != "" {
			curaValues["machine_width"] = w
			curaValues["machine_depth"] = d
		}
	}

	// Determine quality name
	qualityName := "Converted"
	if printSectionName != "" {
		qualityName = strings.TrimPrefix(printSectionName, "print:")
		// Remove @suffix
		if idx := strings.Index(qualityName, " @"); idx > 0 {
			qualityName = qualityName[:idx]
		}
	}

	// Build .inst.cfg output
	var sb strings.Builder
	sb.WriteString("[general]\n")
	sb.WriteString("definition = ankermake_m5\n")
	sb.WriteString(fmt.Sprintf("name = %s\n", qualityName))
	sb.WriteString("version = 4\n\n")

	sb.WriteString("[metadata]\n")
	sb.WriteString("global_quality = True\n")
	sb.WriteString(fmt.Sprintf("quality_type = %s\n", curaQualityType(qualityName)))
	sb.WriteString("setting_version = 20\n")
	sb.WriteString("type = quality\n")
	sb.WriteString("weight = 0\n\n")

	sb.WriteString("[values]\n")
	writeSettings(&sb, curaValues)

	// Sort unmapped
	sort.Strings(unmapped)
	if len(unmapped) > 0 {
		warnings = append(warnings, fmt.Sprintf("%d settings could not be mapped: %s", len(unmapped), strings.Join(unmapped, ", ")))
	}
	warnings = append(warnings, "Cura quality profiles only contain print settings; filament and printer settings need to be configured separately in Cura")

	result := &ConversionResult{
		Content:  sb.String(),
		Filename: strings.TrimSuffix(filename, filepath(filename)) + "_cura.inst.cfg",
		Format:   FormatCura,
		Warnings: warnings,
		Unmapped: unmapped,
		Sections: countSections(sb.String()),
	}
	return result, nil
}

// ─── Parsers ─────────────────────────────────────────────────────────────────

type psSection struct {
	name string
	keys map[string]string
}

func parsePrusaSlicerINI(content string) []psSection {
	var sections []psSection
	var current *psSection
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if current != nil {
				sections = append(sections, *current)
			}
			current = &psSection{name: line[1 : len(line)-1], keys: map[string]string{}}
			continue
		}
		if idx := strings.Index(line, "="); idx > 0 && current != nil {
			current.keys[strings.TrimSpace(line[:idx])] = strings.TrimSpace(line[idx+1:])
		}
	}
	if current != nil {
		sections = append(sections, *current)
	}
	return sections
}

func parseCuraCFG(content string) (map[string]string, map[string]string) {
	values := map[string]string{}
	meta := map[string]string{}
	var inValues bool
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inValues = line == "[values]"
			continue
		}
		if idx := strings.Index(line, "="); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			if inValues {
				values[key] = val
			} else {
				meta[key] = val
			}
		}
	}
	return values, meta
}

func parseCuraJSON(content string) (map[string]string, map[string]string) {
	values := map[string]string{}
	meta := map[string]string{}
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return values, meta
	}
	// Extract metadata
	if md, ok := raw["metadata"].(map[string]interface{}); ok {
		for k, v := range md {
			meta[k] = fmt.Sprintf("%v", v)
		}
	}
	// Extract overrides
	if overrides, ok := raw["overrides"].(map[string]interface{}); ok {
		for key, val := range overrides {
			if settings, ok := val.(map[string]interface{}); ok {
				// Try "value" first, then "default_value"
				if v, ok := settings["value"]; ok {
					values[key] = fmt.Sprintf("%v", v)
				} else if v, ok := settings["default_value"]; ok {
					values[key] = fmt.Sprintf("%v", v)
				}
			}
		}
	}
	return values, meta
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func writeSettings(sb *strings.Builder, settings map[string]string) {
	keys := make([]string, 0, len(settings))
	for k := range settings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sb.WriteString(fmt.Sprintf("%s = %s\n", k, settings[k]))
	}
}

func countSections(content string) int {
	count := 0
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			count++
		}
	}
	return count
}

func filepath(filename string) string {
	if idx := strings.LastIndex(filename, "."); idx > 0 {
		return filename[idx:]
	}
	return ""
}

func isCuraMetaKey(key string) bool {
	metaKeys := map[string]bool{
		"definition": true, "name": true, "version": true,
		"global_quality": true, "quality_type": true, "setting_version": true,
		"type": true, "weight": true,
	}
	return metaKeys[key]
}

func isPSMetaKey(key string) bool {
	metaKeys := map[string]bool{
		" inherits": true, "printer_model": true, "printer_variant": true,
		"printer_settings_id": true, "filament_settings_id": true,
		"print_settings_id": true, "compatible_printers": true,
		"compatible_prints": true, "default_print_profile": true,
		"default_filament_profile": true, "printer_notes": true,
		"filament_notes": true, "print_notes": true,
		"thumbnail": true, "thumbnails": true,
	}
	return metaKeys[key]
}

// Transform functions
func divBy100(s string) string {
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return fmt.Sprintf("%.2f", v/100)
	}
	return s
}

func mulBy100(s string) string {
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return fmt.Sprintf("%.0f", v*100)
	}
	return s
}

func upperCase(s string) string { return strings.ToUpper(s) }
func lowerCase(s string) string { return strings.ToLower(s) }

func boolToVal(s string) string {
	if s == "true" || s == "1" {
		return "1"
	}
	return "0"
}

func valToBool(s string) string {
	if s == "1" || s == "true" {
		return "true"
	}
	return "false"
}

func curaInfillToPS(s string) string {
	m := map[string]string{
		"lines": "line", "grid": "grid", "triangles": "triangles",
		"cubic": "cubic", "cross": "cross", "cross_3d": "cross",
		"concentric": "concentric", "zigzag": "line", "gyroid": "gyroid",
		"quarter_cubic": "cubic", "cubicsubdiv": "cubic",
	}
	if v, ok := m[strings.ToLower(s)]; ok {
		return v
	}
	return s
}

func psInfillToCura(s string) string {
	m := map[string]string{
		"line": "lines", "grid": "grid", "triangles": "triangles",
		"cubic": "cubic", "cross": "cross", "concentric": "concentric",
		"gyroid": "gyroid", "honeycomb": "cubic", "3dhoneycomb": "cubic",
		"adaptivecubic": "cubic", "lineal": "lines",
	}
	if v, ok := m[strings.ToLower(s)]; ok {
		return v
	}
	return s
}

func curaSupportToPS(s string) string {
	switch strings.ToLower(s) {
	case "tree":
		return "organic"
	case "normal":
		return "default"
	default:
		return s
	}
}

func psSupportToCura(s string) string {
	switch strings.ToLower(s) {
	case "organic", "tree":
		return "tree"
	case "default", "normal":
		return "normal"
	default:
		return s
	}
}

func curaQualityType(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "fine") || strings.Contains(lower, "detail"):
		return "fine"
	case strings.Contains(lower, "fast") || strings.Contains(lower, "draft"):
		return "fast"
	case strings.Contains(lower, "extra"):
		return "extra_fast"
	default:
		return "normal"
	}
}

func parseBedShape(shape string) (width, depth string) {
	// Format: "0x0,235x0,235x235,0x235"
	re := regexp.MustCompile(`(\d+)x(\d+)`)
	matches := re.FindAllStringSubmatch(shape, -1)
	if len(matches) >= 2 {
		width = matches[1][1]
	}
	if len(matches) >= 3 {
		depth = matches[2][2]
	}
	return
}
