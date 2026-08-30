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
	FormatOrcaSlicer  Format = "orcaslicer"  // OrcaSlicer / BambuStudio (.json)
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
	// For multi-profile conversions (e.g. flat INI → OrcaSlicer), this contains
	// the individual profiles that can be selected and exported separately.
	Profiles []ProfileOutput `json:"profiles,omitempty"`
}

// ProfileOutput is a single profile within a multi-profile conversion result.
type ProfileOutput struct {
	Type         ProfileType `json:"type"`
	Name         string      `json:"name"`
	Content      string      `json:"content"`
	Filename     string      `json:"filename"`
	SettingCount int         `json:"settingCount"`
}

// ProfileType identifies the kind of slicer profile.
type ProfileType string

const (
	ProfileTypePrint    ProfileType = "print"
	ProfileTypeFilament ProfileType = "filament"
	ProfileTypePrinter  ProfileType = "printer"
)

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
		switch target {
		case FormatPrusaSlicer:
			return curaToPrusaSlicer(content, filename)
		case FormatOrcaSlicer:
			return curaToOrcaSlicer(content, filename)
		}
	case FormatPrusaSlicer:
		switch target {
		case FormatCura:
			return prusaSlicerToCura(content, filename)
		case FormatOrcaSlicer:
			return prusaSlicerToOrcaSlicer(content, filename)
		}
	case FormatOrcaSlicer:
		switch target {
		case FormatPrusaSlicer:
			return orcaSlicerToPrusaSlicer(content, filename)
		case FormatCura:
			return orcaSlicerToCura(content, filename)
		}
	}
	return nil, fmt.Errorf("unsupported conversion: %s → %s", source, target)
}

// DetectFormat tries to determine the profile format from content and filename.
func DetectFormat(content, filename string) Format {
	trimmed := strings.TrimSpace(content)
	lower := strings.ToLower(filename)

	// JSON content → could be Cura .def.json or OrcaSlicer .json
	if strings.HasPrefix(trimmed, "{") {
		// OrcaSlicer JSON has "print_settings_id" or "inherits" or "from" keys
		// Cura .def.json has "overrides" and "metadata" with "manufacturer"
		if strings.Contains(trimmed, "\"print_settings_id\"") ||
			strings.Contains(trimmed, "\"filament_settings_id\"") ||
			strings.Contains(trimmed, "\"printer_settings_id\"") ||
			(strings.Contains(trimmed, "\"inherits\"") && strings.Contains(trimmed, "\"from\"")) {
			return FormatOrcaSlicer
		}
		// Cura .def.json has "overrides" key
		if strings.Contains(trimmed, "\"overrides\"") {
			return FormatCura
		}
		// Default JSON → OrcaSlicer (since Cura .def.json is rarer as an upload)
		if strings.HasSuffix(lower, ".json") {
			return FormatOrcaSlicer
		}
		return FormatCura
	}

	// .inst.cfg files have [general] and [values] sections
	if strings.Contains(content, "[general]") && strings.Contains(content, "[values]") {
		return FormatCura
	}

	// PrusaSlicer INI with sections like [print:...], [filament:...], [printer:...]
	if strings.Contains(content, "[print:") || strings.Contains(content, "[filament:") || strings.Contains(content, "[printer:") {
		return FormatPrusaSlicer
	}

	// Flat PrusaSlicer INI (eufyMake/AnkerMake Studio export) — has key=value
	// pairs with no sections, but contains known PrusaSlicer keys
	if hasPrusaSlicerKeys(content) {
		return FormatPrusaSlicer
	}

	// Check filename extension
	if strings.HasSuffix(lower, ".def.json") || strings.HasSuffix(lower, ".inst.cfg") {
		return FormatCura
	}
	if strings.HasSuffix(lower, ".json") {
		return FormatOrcaSlicer
	}
	if strings.HasSuffix(lower, ".ini") {
		return FormatPrusaSlicer
	}

	// Default: treat as PrusaSlicer INI
	return FormatPrusaSlicer
}

// hasPrusaSlicerKeys checks if content contains known PrusaSlicer setting keys
// without any section headers (flat INI format from eufyMake/AnkerMake Studio).
func hasPrusaSlicerKeys(content string) bool {
	knownKeys := []string{
		"layer_height", "fill_density", "perimeters", "nozzle_diameter",
		"filament_diameter", "bed_temperature", "retract_length",
		"print_speed", "infill_speed", "perimeter_speed",
		"printer_model", "filament_type", "bed_shape",
	}
	count := 0
	for _, key := range knownKeys {
		if strings.Contains(content, key+" =") || strings.Contains(content, key+"=") {
			count++
		}
	}
	return count >= 3
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

	// Parse PrusaSlicer INI — could be sectioned ([print:...]) or flat (eufyMake export)
	sections := parsePrusaSlicerINI(content)

	allSettings := map[string]string{}
	var printSectionName string

	if len(sections) > 0 {
		// Sectioned INI — collect settings from all sections
		for _, sec := range sections {
			switch {
			case strings.HasPrefix(sec.name, "print:"):
				printSectionName = sec.name
				for k, v := range sec.keys {
					allSettings[k] = v
				}
			case strings.HasPrefix(sec.name, "filament:"):
				for k, v := range sec.keys {
					allSettings[k] = v
				}
			case strings.HasPrefix(sec.name, "printer:"):
				for k, v := range sec.keys {
					allSettings[k] = v
				}
			}
		}
	} else {
		// Flat INI (eufyMake/AnkerMake Studio export) — all key=value pairs
		allSettings = parseFlatINI(content)
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

// parseFlatINI parses a flat INI file with no section headers (eufyMake/AnkerMake
// Studio config export). All key=value pairs are returned in a single map.
func parseFlatINI(content string) map[string]string {
	result := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		// Skip section headers (shouldn't be any in flat INI, but just in case)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			continue
		}
		if idx := strings.Index(line, "="); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			// Strip surrounding quotes
			if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
				val = val[1 : len(val)-1]
			}
			result[key] = val
		}
	}
	return result
}

// parseOrcaSlicerJSON parses an OrcaSlicer/BambuStudio JSON profile.
// These profiles can be print, filament, or printer profiles.
// They contain setting keys directly in the JSON root, plus metadata
// like "inherits", "name", "from", "version", etc.
func parseOrcaSlicerJSON(content string) (map[string]string, map[string]string) {
	values := map[string]string{}
	meta := map[string]string{}
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return values, meta
	}
	// Metadata keys that are not slicer settings
	metaKeys := map[string]bool{
		"from": true, "inherits": true, "name": true, "version": true,
		"print_settings_id": true, "filament_settings_id": true,
		"printer_settings_id": true, "print_extruder_id": true,
		"print_extruder_variant": true, "enable": true,
	}
	for key, val := range raw {
		if metaKeys[key] {
			meta[key] = fmt.Sprintf("%v", val)
			continue
		}
		// Convert value to string
		switch v := val.(type) {
		case string:
			values[key] = v
		case float64:
			if v == float64(int(v)) {
				values[key] = strconv.Itoa(int(v))
			} else {
				values[key] = strconv.FormatFloat(v, 'f', -1, 64)
			}
		case bool:
			if v {
				values[key] = "1"
			} else {
				values[key] = "0"
			}
		case []interface{}:
			// Arrays (like print_extruder_id) — join with comma
			parts := make([]string, 0, len(v))
			for _, item := range v {
				parts = append(parts, fmt.Sprintf("%v", item))
			}
			values[key] = strings.Join(parts, ",")
		default:
			values[key] = fmt.Sprintf("%v", val)
		}
	}
	return values, meta
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
func filepathExt(filename string) string {
	if idx := strings.LastIndex(filename, "."); idx > 0 {
		return filename[idx:]
	}
	return ""
}

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

// ─── OrcaSlicer JSON conversions ─────────────────────────────────────────────
// OrcaSlicer/BambuStudio uses the same setting names as PrusaSlicer, but the
// profile is stored as JSON instead of INI. So conversion between OrcaSlicer
// and PrusaSlicer is mostly a format change (JSON ↔ INI), while conversion
// between OrcaSlicer and Cura uses the same mapping as PrusaSlicer ↔ Cura.

func orcaSlicerToPrusaSlicer(content string, filename string) (*ConversionResult, error) {
	warnings := []string{}
	unmapped := []string{}

	values, meta := parseOrcaSlicerJSON(content)

	if len(values) == 0 {
		warnings = append(warnings, "No settings found in OrcaSlicer JSON. This profile may only contain overrides (inherits from a base profile). The base profile settings are not included in the export.")
	}

	// Determine profile name
	profileName := meta["name"]
	if profileName == "" {
		profileName = meta["print_settings_id"]
	}
	if profileName == "" {
		profileName = "Converted"
	}

	// Determine which type of profile this is
	var profileType string
	if meta["print_settings_id"] != "" || strings.Contains(filename, "print") {
		profileType = "print"
	} else if meta["filament_settings_id"] != "" || strings.Contains(filename, "filament") {
		profileType = "filament"
	} else if meta["printer_settings_id"] != "" || strings.Contains(filename, "printer") {
		profileType = "printer"
	} else {
		profileType = "print" // default
	}

	// Build INI output
	var sb strings.Builder
	sb.WriteString("# Generated by OpenPolyPrint profile converter\n")
	sb.WriteString("# Source: OrcaSlicer/BambuStudio JSON format\n")
	sb.WriteString("# Target: PrusaSlicer/eufyMake Studio INI format\n")
	if meta["inherits"] != "" {
		sb.WriteString(fmt.Sprintf("# Inherits: %s\n", meta["inherits"]))
	}
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("[%s:%s]\n", profileType, profileName))
	if meta["inherits"] != "" {
		sb.WriteString(fmt.Sprintf("inherits = %s\n", meta["inherits"]))
	}

	// Sort keys for stable output
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		// Skip OrcaSlicer-specific meta keys
		if isOrcaMetaKey(k) {
			continue
		}
		sb.WriteString(fmt.Sprintf("%s = %s\n", k, values[k]))
	}

	if len(unmapped) > 0 {
		warnings = append(warnings, fmt.Sprintf("%d settings could not be mapped: %s", len(unmapped), strings.Join(unmapped, ", ")))
	}

	result := &ConversionResult{
		Content:  sb.String(),
		Filename: strings.TrimSuffix(filename, filepathExt(filename)) + "_prusaslicer.ini",
		Format:   FormatPrusaSlicer,
		Warnings: warnings,
		Unmapped: unmapped,
		Sections: countSections(sb.String()),
	}
	return result, nil
}

func prusaSlicerToOrcaSlicer(content string, filename string) (*ConversionResult, error) {
	warnings := []string{}

	// Parse PrusaSlicer INI — could be sectioned or flat
	sections := parsePrusaSlicerINI(content)
	allSettings := map[string]string{}
	var profileName string
	isFlat := false

	if len(sections) > 0 {
		for _, sec := range sections {
			if profileName == "" && strings.Contains(sec.name, ":") {
				profileName = strings.SplitN(sec.name, ":", 2)[1]
			}
			for k, v := range sec.keys {
				allSettings[k] = v
			}
		}
	} else {
		allSettings = parseFlatINI(content)
		isFlat = true
	}

	if profileName == "" {
		profileName = strings.TrimSuffix(filename, filepathExt(filename))
		if profileName == "" {
			profileName = "Converted"
		}
	}

	// For flat INI (eufyMake export), split into separate print/filament/printer
	// profiles since OrcaSlicer expects separate JSON files for each type.
	if isFlat {
		return prusaSlicerFlatToOrcaSlicerMulti(allSettings, profileName, filename, warnings)
	}

	// For sectioned INI, produce a single JSON (existing behavior)
	jsonMap := map[string]interface{}{
		"from":              "OpenPolyPrint Converter",
		"name":              profileName,
		"version":           "2.4.0.1",
		"print_settings_id": profileName,
	}

	mapped := 0
	for k, v := range allSettings {
		if isPSMetaKey(k) || isOrcaMetaKey(k) {
			continue
		}
		jsonMap[k] = stringToJSONValue(v)
		mapped++
	}

	if mapped == 0 {
		warnings = append(warnings, "No settings were mapped. Check that the source file contains valid PrusaSlicer settings.")
	}

	jsonData, err := json.MarshalIndent(jsonMap, "", "\t")
	if err != nil {
		return nil, fmt.Errorf("failed to encode JSON: %w", err)
	}

	result := &ConversionResult{
		Content:  string(jsonData),
		Filename: strings.TrimSuffix(filename, filepathExt(filename)) + "_orcaslicer.json",
		Format:   FormatOrcaSlicer,
		Warnings: warnings,
	}
	return result, nil
}

// prusaSlicerFlatToOrcaSlicerMulti splits a flat INI (eufyMake/AnkerMake Studio
// export) into separate OrcaSlicer JSON profiles for print, filament, and
// printer settings. This is needed because OrcaSlicer imports separate JSON
// files for each profile type, not a combined file.
func prusaSlicerFlatToOrcaSlicerMulti(allSettings map[string]string, profileName, filename string, warnings []string) (*ConversionResult, error) {
	printSettings, filamentSettings, printerSettings := categorizePrusaSlicerSettings(allSettings)

	// Extract names from the settings if available
	printName := profileName
	if v, ok := allSettings["print_settings_id"]; ok && v != "" {
		printName = v
	}
	filamentName := profileName
	if v, ok := allSettings["filament_settings_id"]; ok && v != "" {
		filamentName = v
	} else if v, ok := allSettings["default_filament_profile"]; ok && v != "" {
		filamentName = v
	}
	printerName := profileName
	if v, ok := allSettings["printer_settings_id"]; ok && v != "" {
		printerName = v
	} else if v, ok := allSettings["printer_model"]; ok && v != "" {
		printerName = v
	}

	baseFilename := strings.TrimSuffix(filename, filepathExt(filename))
	var profiles []ProfileOutput

	// Print profile
	if len(printSettings) > 0 {
		jsonMap := map[string]interface{}{
			"from":              "OpenPolyPrint Converter",
			"name":              printName,
			"version":           "2.4.0.1",
			"print_settings_id": printName,
		}
		for k, v := range printSettings {
			if isPSMetaKey(k) || isOrcaMetaKey(k) {
				continue
			}
			jsonMap[k] = stringToJSONValue(v)
		}
		jsonData, _ := json.MarshalIndent(jsonMap, "", "\t")
		profiles = append(profiles, ProfileOutput{
			Type:         ProfileTypePrint,
			Name:         printName,
			Content:      string(jsonData),
			Filename:     baseFilename + "_print.json",
			SettingCount: len(printSettings),
		})
	}

	// Filament profile
	if len(filamentSettings) > 0 {
		jsonMap := map[string]interface{}{
			"from":                 "OpenPolyPrint Converter",
			"name":                 filamentName,
			"version":              "2.4.0.1",
			"filament_settings_id": filamentName,
		}
		for k, v := range filamentSettings {
			if isPSMetaKey(k) || isOrcaMetaKey(k) {
				continue
			}
			jsonMap[k] = stringToJSONValue(v)
		}
		jsonData, _ := json.MarshalIndent(jsonMap, "", "\t")
		profiles = append(profiles, ProfileOutput{
			Type:         ProfileTypeFilament,
			Name:         filamentName,
			Content:      string(jsonData),
			Filename:     baseFilename + "_filament.json",
			SettingCount: len(filamentSettings),
		})
	}

	// Printer profile
	if len(printerSettings) > 0 {
		jsonMap := map[string]interface{}{
			"from":                "OpenPolyPrint Converter",
			"name":                printerName,
			"version":             "2.4.0.1",
			"printer_settings_id": printerName,
		}
		for k, v := range printerSettings {
			if isPSMetaKey(k) || isOrcaMetaKey(k) {
				continue
			}
			jsonMap[k] = stringToJSONValue(v)
		}
		jsonData, _ := json.MarshalIndent(jsonMap, "", "\t")
		profiles = append(profiles, ProfileOutput{
			Type:         ProfileTypePrinter,
			Name:         printerName,
			Content:      string(jsonData),
			Filename:     baseFilename + "_printer.json",
			SettingCount: len(printerSettings),
		})
	}

	if len(profiles) == 0 {
		warnings = append(warnings, "No settings found to split into profiles.")
	} else {
		warnings = append(warnings, fmt.Sprintf("Split into %d profiles: %s. Import each JSON file separately in OrcaSlicer (File → Import → Import config).", len(profiles), profileTypesSummary(profiles)))
	}

	// Use the first profile as the default content for backwards compat
	defaultContent := ""
	defaultFilename := baseFilename + "_orcaslicer.json"
	if len(profiles) > 0 {
		defaultContent = profiles[0].Content
		defaultFilename = profiles[0].Filename
	}

	return &ConversionResult{
		Content:  defaultContent,
		Filename: defaultFilename,
		Format:   FormatOrcaSlicer,
		Warnings: warnings,
		Profiles: profiles,
	}, nil
}

// categorizePrusaSlicerSettings splits a flat map of PrusaSlicer settings into
// print, filament, and printer categories based on key name prefixes and
// known setting categorizations from PrusaSlicer/OrcaSlicer source.
func categorizePrusaSlicerSettings(all map[string]string) (print, filament, printer map[string]string) {
	print = map[string]string{}
	filament = map[string]string{}
	printer = map[string]string{}

	for k, v := range all {
		cat := categorizeSetting(k)
		switch cat {
		case "filament":
			filament[k] = v
		case "printer":
			printer[k] = v
		default:
			print[k] = v
		}
	}
	return
}

// categorizeSetting determines which profile type a PrusaSlicer setting belongs to.
// Based on PrusaSlicer/OrcaSlicer setting category definitions.
func categorizeSetting(key string) string {
	// Filament settings — prefixed with filament_ or related to material/temperature
	filamentPrefixes := []string{
		"filament_", "default_filament_profile",
	}
	filamentExact := map[string]bool{
		"temperature": true, "bed_temperature": true,
		"first_layer_temperature": true, "first_layer_bed_temperature": true,
		"cooling": true, "max_fan_speed": true, "min_fan_speed": true,
		"disable_fan_first_layers": true, "fan_always_on": true,
		"fan_below_layer_time": true, "bridge_fan_speed": true,
		"full_fan_speed_layer": true, "enable_dynamic_fan_speeds": true,
		"overhang_fan_speed_0": true, "overhang_fan_speed_1": true,
		"overhang_fan_speed_2": true, "overhang_fan_speed_3": true,
		"idle_temperature": true, "standby_temperature_delta": true,
		"high_current_on_filament_swap": true,
		"autoemit_temperature_commands": true,
		"filament_cooling_final_speed":  true, "filament_cooling_initial_speed": true,
		"filament_cooling_moves": true, "filament_load_time": true,
		"filament_loading_speed": true, "filament_loading_speed_start": true,
		"filament_unload_time": true, "filament_unloading_speed": true,
		"filament_unloading_speed_start": true,
		"filament_max_volumetric_speed":  true, "filament_minimal_purge_on_wipe_tower": true,
		"filament_ramming_parameters": true, "filament_retract_before_travel": true,
		"filament_retract_before_wipe": true, "filament_retract_layer_change": true,
		"filament_retract_length": true, "filament_retract_lift": true,
		"filament_retract_lift_above": true, "filament_retract_lift_below": true,
		"filament_retract_restart_extra": true, "filament_retract_speed": true,
		"filament_soluble": true, "filament_spool_weight": true,
		"filament_toolchange_delay": true, "filament_wipe": true,
		"filament_deretract_speed": true, "filament_cost": true,
		"filament_density": true, "filament_colour": true,
		"filament_notes": true, "filament_vendor": true,
		"filament_settings_id": true, "end_filament_gcode": true,
		"start_filament_gcode": true,
		"extruder_colour":      true, "extruder_offset": true,
		"max_volumetric_speed": true, "max_volumetric_extrusion_rate_slope_negative": true,
		"max_volumetric_extrusion_rate_slope_positive": true,
		"single_extruder_multi_material":               true, "single_extruder_multi_material_priming": true,
		"extrusion_multiplier": true,
	}

	// Printer settings — prefixed with machine_, printer_, or related to hardware
	printerPrefixes := []string{
		"machine_", "printer_", "bed_", "nozzle_",
	}
	printerExact := map[string]bool{
		"bed_shape": true, "bed_custom_model": true, "bed_custom_texture": true,
		"max_print_height": true, "nozzle_diameter": true,
		"printer_model": true, "printer_vendor": true, "printer_technology": true,
		"printer_variant": true, "printer_settings_id": true,
		"printer_notes": true, "physical_printer_settings_id": true,
		"gcode_flavor": true, "gcode_resolution": true, "gcode_substitutions": true,
		"gcode_comments": true, "gcode_label_objects": true,
		"start_gcode": true, "end_gcode": true,
		"before_layer_gcode": true, "layer_gcode": true,
		"toolchange_gcode": true, "between_objects_gcode": true,
		"color_change_gcode": true, "pause_print_gcode": true,
		"template_custom_gcode":     true,
		"extruder_clearance_height": true, "extruder_clearance_radius": true,
		"extrusion_axis": true, "extruder_count": true,
		"use_firmware_retraction": true, "use_relative_e_distances": true,
		"use_volumetric_e": true, "variable_layer_height": true,
		"silent_mode": true, "remaining_times": true,
		"host_type": true, "print_host": true,
		"printhost_apikey": true, "printhost_cafile": true,
		"thumbnails": true, "thumbnails_format": true,
		"output_filename_format": true,
		"default_print_profile":  true,
		"threads":                true, "z_offset": true,
		"xy_hole_compensation": true, "xy_size_compensation": true,
		"hole_offset": true, "elefant_foot_compensation": true,
		"lift_type": true, "travel_speed_z": true,
		"machine_limits_usage": true,
		"slice_closing_radius": true, "slicing_mode": true,
		"mmu_segmented_region_max_width": true,
		"parking_pos_retraction":         true,
		"wiping_volumes_extruders":       true, "wiping_volumes_matrix": true,
		"compatible_printers_condition_cummulative": true,
		"compatible_prints_condition_cummulative":   true,
		"inherits_cummulative":                      true,
		"notes":                                     true,
		"colorprint_heights":                        true,
		"post_process":                              true,
		"enable_arc_fitting":                        true,
		"make_overhang_printable":                   true, "make_overhang_printable_angle": true,
		"make_overhang_printable_hole_size": true,
		"slow_down_layers":                  true,
		"jerk_enable":                       true, "jerk_first_layer": true,
		"jerk_infill": true, "jerk_inner_wall": true, "jerk_outer_wall": true,
		"jerk_print": true, "jerk_skirt_brim": true, "jerk_top_bottom": true,
		"jerk_top_surface": true, "jerk_travel": true,
		"jerk_e_enable": true, "jerk_e_infill": true, "jerk_e_inner_wall": true,
		"jerk_e_outer_wall": true, "jerk_e_print": true, "jerk_e_skin": true,
		"jerk_e_skirt_brim": true, "jerk_e_support": true,
	}

	// Check filament
	for _, prefix := range filamentPrefixes {
		if strings.HasPrefix(key, prefix) {
			return "filament"
		}
	}
	if filamentExact[key] {
		return "filament"
	}

	// Check printer
	for _, prefix := range printerPrefixes {
		if strings.HasPrefix(key, prefix) {
			return "printer"
		}
	}
	if printerExact[key] {
		return "printer"
	}

	// Everything else is a print setting
	return "print"
}

// stringToJSONValue converts a string value to the appropriate JSON type
// (bool, int, float, or string).
func stringToJSONValue(v string) interface{} {
	if v == "1" || v == "true" {
		return true
	}
	if v == "0" || v == "false" {
		return false
	}
	if i, err := strconv.Atoi(v); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return f
	}
	return v
}

func profileTypesSummary(profiles []ProfileOutput) string {
	parts := make([]string, 0, len(profiles))
	for _, p := range profiles {
		parts = append(parts, string(p.Type))
	}
	return strings.Join(parts, ", ")
}

func orcaSlicerToCura(content string, filename string) (*ConversionResult, error) {
	warnings := []string{}
	unmapped := []string{}

	values, meta := parseOrcaSlicerJSON(content)

	if len(values) == 0 {
		warnings = append(warnings, "No settings found in OrcaSlicer JSON. This profile may only contain overrides (inherits from a base profile). The base profile settings are not included in the export.")
	}

	// OrcaSlicer uses same setting names as PrusaSlicer, so reuse psToCuraMap
	curaValues := map[string]string{}

	for psKey, psVal := range values {
		mapping, ok := psToCuraMap[psKey]
		if !ok {
			if isPSMetaKey(psKey) || isOrcaMetaKey(psKey) {
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

	// Parse bed_shape
	if bedShape, ok := values["bed_shape"]; ok {
		w, d := parseBedShape(bedShape)
		if w != "" && d != "" {
			curaValues["machine_width"] = w
			curaValues["machine_depth"] = d
		}
	}

	qualityName := meta["name"]
	if qualityName == "" {
		qualityName = "Converted"
	}

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

	sort.Strings(unmapped)
	if len(unmapped) > 0 {
		warnings = append(warnings, fmt.Sprintf("%d settings could not be mapped: %s", len(unmapped), strings.Join(unmapped, ", ")))
	}
	warnings = append(warnings, "Cura quality profiles only contain print settings; filament and printer settings need to be configured separately in Cura")

	result := &ConversionResult{
		Content:  sb.String(),
		Filename: strings.TrimSuffix(filename, filepathExt(filename)) + "_cura.inst.cfg",
		Format:   FormatCura,
		Warnings: warnings,
		Unmapped: unmapped,
		Sections: countSections(sb.String()),
	}
	return result, nil
}

func curaToOrcaSlicer(content string, filename string) (*ConversionResult, error) {
	warnings := []string{}
	unmapped := []string{}

	// Parse Cura content
	var curaValues map[string]string
	var curaMeta map[string]string

	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "{") {
		curaValues, curaMeta = parseCuraJSON(content)
	} else {
		curaValues, curaMeta = parseCuraCFG(content)
	}

	// Convert Cura values to PrusaSlicer/OrcaSlicer setting names
	// (OrcaSlicer uses PrusaSlicer setting names)
	orcaValues := map[string]string{}

	for curaKey, curaVal := range curaValues {
		mapping, ok := curaToPSMap[curaKey]
		if !ok {
			if isCuraMetaKey(curaKey) {
				continue
			}
			unmapped = append(unmapped, curaKey)
			continue
		}

		// Skip special placeholder keys
		if strings.HasPrefix(mapping.psKey, "_") {
			continue
		}

		val := curaVal
		if mapping.transform != nil {
			val = mapping.transform(val)
		}
		orcaValues[mapping.psKey] = val
	}

	profileName := curaMeta["name"]
	if profileName == "" {
		profileName = "Converted"
	}

	// Build JSON
	jsonMap := map[string]interface{}{
		"from":              "OpenPolyPrint Converter",
		"name":              profileName,
		"version":           "2.4.0.1",
		"print_settings_id": profileName,
	}

	for k, v := range orcaValues {
		if v == "1" || v == "true" {
			jsonMap[k] = true
		} else if v == "0" || v == "false" {
			jsonMap[k] = false
		} else if i, err := strconv.Atoi(v); err == nil {
			jsonMap[k] = i
		} else if f, err := strconv.ParseFloat(v, 64); err == nil {
			jsonMap[k] = f
		} else {
			jsonMap[k] = v
		}
	}

	jsonData, err := json.MarshalIndent(jsonMap, "", "\t")
	if err != nil {
		return nil, fmt.Errorf("failed to encode JSON: %w", err)
	}

	sort.Strings(unmapped)
	if len(unmapped) > 0 {
		warnings = append(warnings, fmt.Sprintf("%d settings could not be mapped: %s", len(unmapped), strings.Join(unmapped, ", ")))
	}

	result := &ConversionResult{
		Content:  string(jsonData),
		Filename: strings.TrimSuffix(filename, filepathExt(filename)) + "_orcaslicer.json",
		Format:   FormatOrcaSlicer,
		Warnings: warnings,
		Unmapped: unmapped,
	}
	return result, nil
}

func isOrcaMetaKey(key string) bool {
	metaKeys := map[string]bool{
		"from": true, "inherits": true, "name": true, "version": true,
		"print_settings_id": true, "filament_settings_id": true,
		"printer_settings_id": true, "print_extruder_id": true,
		"print_extruder_variant": true, "enable": true,
		"anker_colour_id": true, "anker_filament_id": true,
	}
	return metaKeys[key]
}
