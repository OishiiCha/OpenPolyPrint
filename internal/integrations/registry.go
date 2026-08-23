package integrations

// Field is a single configuration field for an integration.
type Field struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Type        string `json:"type"`
	Placeholder string `json:"placeholder"`
}

// Integration describes a third-party service that can be wired into OpenPolyPrint.
type Integration struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Category     string  `json:"category"`
	Icon         string  `json:"icon"`
	Color        string  `json:"color"`
	Description  string  `json:"description"`
	LongDesc     string  `json:"longDesc"`
	URL          string  `json:"url"`
	URLLabel     string  `json:"urlLabel"`
	AlwaysActive bool    `json:"alwaysActive"`
	Fields       []Field `json:"fields"`
}

// Registry is the list of available integrations imported from the embedded project.
var Registry = []Integration{
	{
		ID:          "homeassistant",
		Name:        "Home Assistant",
		Category:    "Smart Home",
		Description: "Real-time MQTT sensor + camera via go2rtc",
		LongDesc:    "Integrate your AnkerMake printer with Home Assistant via MQTT. Get real-time print status, temperature sensors, and camera feeds.",
		URL:         "https://github.com/sondregronas/ankermake-hass-component",
		URLLabel:    "HASS Component",
		Fields: []Field{
			{ID: "mqtt_host", Label: "MQTT Broker Host", Type: "text", Placeholder: "homeassistant.local"},
			{ID: "mqtt_port", Label: "MQTT Port", Type: "number", Placeholder: "1883"},
			{ID: "mqtt_user", Label: "MQTT Username", Type: "text", Placeholder: "(optional)"},
			{ID: "mqtt_pass", Label: "MQTT Password", Type: "password", Placeholder: "(optional)"},
		},
	},
	{
		ID:          "go2rtc",
		Name:        "go2rtc",
		Category:    "Camera",
		Description: "Stream M5 camera as WebRTC/RTSP",
		LongDesc:    "go2rtc converts the M5 built-in camera stream to WebRTC, RTSP, MJPEG, and other formats. Enables camera access in Home Assistant, Frigate, and other NVR systems.",
		URL:         "https://github.com/AlexxIT/go2rtc",
		URLLabel:    "go2rtc GitHub",
		Fields: []Field{
			{ID: "stream_url", Label: "Source Stream URL", Type: "url", Placeholder: "http://openpolyprint-ip:4470/video"},
			{ID: "rtsp_port", Label: "RTSP Port", Type: "number", Placeholder: "8554"},
		},
	},
	{
		ID:          "obico",
		Name:        "Obico",
		Category:    "AI Monitoring",
		Description: "AI failure detection + remote access",
		LongDesc:    "Obico uses AI to monitor your prints for failures and sends alerts. Also provides secure remote access to your printer from anywhere.",
		URL:         "https://obico.io",
		URLLabel:    "obico.io",
		Fields: []Field{
			{ID: "obico_url", Label: "Obico Server URL", Type: "url", Placeholder: "https://app.obico.io"},
			{ID: "obico_token", Label: "API Token", Type: "password", Placeholder: "Get from Obico dashboard"},
		},
	},
	{
		ID:          "octoeverywhere",
		Name:        "OctoEverywhere",
		Category:    "Remote Access",
		Description: "Secure remote access from anywhere",
		LongDesc:    "OctoEverywhere provides secure remote access to your printer with end-to-end encryption. No VPN needed. Free tier available with unlimited printing.",
		URL:         "https://octoeverywhere.com",
		URLLabel:    "octoeverywhere.com",
		Fields: []Field{
			{ID: "oe_email", Label: "Account Email", Type: "email", Placeholder: "you@example.com"},
		},
	},
	{
		ID:          "telegram",
		Name:        "Telegram Bot",
		Category:    "Notifications",
		Description: "Print notifications + snapshots",
		LongDesc:    "Receive Telegram notifications when prints start, complete, or fail. Includes camera snapshots. Create a bot via @BotFather and get your chat ID from @userinfobot.",
		URL:         "https://t.me/BotFather",
		URLLabel:    "Create a Bot",
		Fields: []Field{
			{ID: "token", Label: "Bot Token", Type: "password", Placeholder: "123456:ABC-DEF..."},
			{ID: "chat_id", Label: "Chat ID", Type: "text", Placeholder: "123456789"},
		},
	},
	{
		ID:          "discord",
		Name:        "Discord Webhook",
		Category:    "Notifications",
		Description: "Print complete/fail alerts",
		LongDesc:    "Send print notifications to a Discord channel via webhook. Create a webhook in your server: Channel Settings → Integrations → Webhooks → New Webhook.",
		URL:         "https://support.discord.com/hc/en-us/articles/228383668-Intro-to-Webhooks",
		URLLabel:    "Webhook Guide",
		Fields: []Field{
			{ID: "webhook_url", Label: "Webhook URL", Type: "url", Placeholder: "https://discord.com/api/webhooks/..."},
		},
	},
	{
		ID:           "prusaslicer",
		Name:         "PrusaSlicer",
		Category:     "Slicers",
		Description:  "Direct upload via OctoPrint API",
		LongDesc:     "PrusaSlicer can upload G-code directly to OpenPolyPrint using the OctoPrint protocol. Configure a Physical Printer with the OpenPolyPrint address.",
		URL:          "https://www.prusa3d.com/page/prusaslicer_424/",
		URLLabel:     "Download PrusaSlicer",
		AlwaysActive: true,
		Fields:       []Field{},
	},
	{
		ID:           "orcaslicer",
		Name:         "OrcaSlicer",
		Category:     "Slicers",
		Description:  "Direct upload via OctoPrint API",
		LongDesc:     "OrcaSlicer supports direct upload to OpenPolyPrint via the OctoPrint protocol. Add a physical printer in OrcaSlicer settings with the OpenPolyPrint address.",
		URL:          "https://github.com/SoftFever/OrcaSlicer",
		URLLabel:     "Download OrcaSlicer",
		AlwaysActive: true,
		Fields:       []Field{},
	},
	{
		ID:          "cura",
		Name:        "Cura",
		Category:    "Slicers",
		Description: "Upload via OctoPrint plugin",
		LongDesc:    "UltiMaker Cura can upload to OpenPolyPrint using the OctoPrint plugin. Install the OctoPrint plugin from Cura Marketplace and configure with the OpenPolyPrint address.",
		URL:         "https://ultimaker.com/software/ultimaker-cura/",
		URLLabel:    "Download Cura",
		Fields:      []Field{},
	},
	{
		ID:          "n8n",
		Name:        "n8n / Zapier",
		Category:    "Automation",
		Description: "Automate with webhooks + REST API",
		LongDesc:    "Use n8n or Zapier to automate workflows based on print events. Trigger webhooks on print start/complete/fail, or poll the OpenPolyPrint REST API for status.",
		URL:         "https://n8n.io",
		URLLabel:    "n8n.io",
		Fields: []Field{
			{ID: "webhook_url", Label: "Webhook URL", Type: "url", Placeholder: "https://n8n.example.com/webhook/print"},
			{ID: "events", Label: "Events (comma-separated)", Type: "text", Placeholder: "start,complete,fail"},
		},
	},
	{
		ID:          "timelapse",
		Name:        "Timelapse",
		Category:    "Camera",
		Description: "Auto-capture during print (requires ffmpeg)",
		LongDesc:    "Automatically capture timelapse videos during prints using the built-in camera. Requires ffmpeg to be installed on the server. Snapshots are taken at regular intervals and compiled into a video.",
		URL:         "https://ffmpeg.org",
		URLLabel:    "ffmpeg.org",
		Fields: []Field{
			{ID: "interval", Label: "Capture Interval (seconds)", Type: "number", Placeholder: "30"},
			{ID: "output_dir", Label: "Output Directory", Type: "text", Placeholder: "./timelapses"},
		},
	},
}
