// Package orcaembed embeds the OrcaSlicer bridge plugin so the server can
// serve it at GET /api/orca/plugin for installation on other machines
// (OrcaSlicer typically runs on a different device than OpenPolyPrint).
package orcaembed

import _ "embed"

//go:embed OpenPolyPrintBridge/plugin.py
var PluginScript string

// PluginFolder is the folder name the plugin must be installed under in
// OrcaSlicer's orca_plugins directory.
const PluginFolder = "OpenPolyPrintBridge"

// PluginVersion mirrors the version in the plugin's PEP 723 metadata.
const PluginVersion = "1.1.0"
