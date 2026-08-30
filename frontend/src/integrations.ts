import type { ComponentType, CSSProperties } from 'react'
import {
  Eye,
  Globe,
  Home,
  MessageCircle,
  Radio,
  Send,
  Settings,
  Settings2,
  SlidersHorizontal,
  Video,
  Zap,
} from 'lucide-react'

export interface IntegrationField {
  id: string
  label: string
  type: string
  placeholder: string
}

export interface Integration {
  id: string
  name: string
  category: string
  icon: string
  color: string
  description: string
  longDesc: string
  url: string
  urlLabel: string
  alwaysActive: boolean
  fields: IntegrationField[]
}

export const integrationIcons: Record<string, ComponentType<{ className?: string; style?: CSSProperties }>> = {
  home: Home,
  radio: Radio,
  eye: Eye,
  globe: Globe,
  send: Send,
  message: MessageCircle,
  settings: Settings,
  settings2: Settings2,
  sliders: SlidersHorizontal,
  zap: Zap,
  video: Video,
}

export const integrations: Integration[] = [
  {
    id: 'homeassistant',
    name: 'Home Assistant',
    category: 'Smart Home',
    icon: 'home',
    color: '#0d6efd',
    description: 'Real-time MQTT sensor + camera via go2rtc',
    longDesc: 'Integrate your AnkerMake printer with Home Assistant via MQTT. Get real-time print status, temperature sensors, and camera feeds.',
    url: 'https://github.com/sondregronas/ankermake-hass-component',
    urlLabel: 'HASS Component',
    alwaysActive: false,
    fields: [
      { id: 'mqtt_host', label: 'MQTT Broker Host', type: 'text', placeholder: 'homeassistant.local' },
      { id: 'mqtt_port', label: 'MQTT Port', type: 'number', placeholder: '1883' },
      { id: 'mqtt_user', label: 'MQTT Username', type: 'text', placeholder: '(optional)' },
      { id: 'mqtt_pass', label: 'MQTT Password', type: 'password', placeholder: '(optional)' },
    ],
  },
  {
    id: 'go2rtc',
    name: 'go2rtc',
    category: 'Camera',
    icon: 'radio',
    color: '#0dcaf0',
    description: 'Stream M5 camera as WebRTC/RTSP',
    longDesc: 'go2rtc converts the M5 built-in camera stream to WebRTC, RTSP, MJPEG, and other formats. Enables camera access in Home Assistant, Frigate, and other NVR systems.',
    url: 'https://github.com/AlexxIT/go2rtc',
    urlLabel: 'go2rtc GitHub',
    alwaysActive: false,
    fields: [
      { id: 'stream_url', label: 'Source Stream URL', type: 'url', placeholder: 'http://openpolyprint-ip:4470/video' },
      { id: 'rtsp_port', label: 'RTSP Port', type: 'number', placeholder: '8554' },
    ],
  },
  {
    id: 'obico',
    name: 'Obico',
    category: 'AI Monitoring',
    icon: 'eye',
    color: '#7c3aed',
    description: 'AI failure detection + remote access',
    longDesc: 'Obico uses AI to monitor your prints for failures and sends alerts. Also provides secure remote access to your printer from anywhere.',
    url: 'https://obico.io',
    urlLabel: 'obico.io',
    alwaysActive: false,
    fields: [
      { id: 'obico_url', label: 'Obico Server URL', type: 'url', placeholder: 'https://app.obico.io' },
      { id: 'obico_token', label: 'API Token', type: 'password', placeholder: 'Get from Obico dashboard' },
    ],
  },
  {
    id: 'octoeverywhere',
    name: 'OctoEverywhere',
    category: 'Remote Access',
    icon: 'globe',
    color: '#198754',
    description: 'Secure remote access from anywhere',
    longDesc: 'OctoEverywhere provides secure remote access to your printer with end-to-end encryption. No VPN needed. Free tier available with unlimited printing.',
    url: 'https://octoeverywhere.com',
    urlLabel: 'octoeverywhere.com',
    alwaysActive: false,
    fields: [{ id: 'oe_email', label: 'Account Email', type: 'email', placeholder: 'you@example.com' }],
  },
  {
    id: 'telegram',
    name: 'Telegram Bot',
    category: 'Notifications',
    icon: 'send',
    color: '#0088cc',
    description: 'Print notifications + snapshots',
    longDesc: 'Receive Telegram notifications when prints start, complete, or fail. Includes camera snapshots. Create a bot via @BotFather and get your chat ID from @userinfobot.',
    url: 'https://t.me/BotFather',
    urlLabel: 'Create a Bot',
    alwaysActive: false,
    fields: [
      { id: 'token', label: 'Bot Token', type: 'password', placeholder: '123456:ABC-DEF...' },
      { id: 'chat_id', label: 'Chat ID', type: 'text', placeholder: '123456789' },
    ],
  },
  {
    id: 'discord',
    name: 'Discord Webhook',
    category: 'Notifications',
    icon: 'message',
    color: '#5865F2',
    description: 'Print complete/fail alerts',
    longDesc: 'Send print notifications to a Discord channel via webhook. Create a webhook in your server: Channel Settings → Integrations → Webhooks → New Webhook.',
    url: 'https://support.discord.com/hc/en-us/articles/228383668-Intro-to-Webhooks',
    urlLabel: 'Webhook Guide',
    alwaysActive: false,
    fields: [{ id: 'webhook_url', label: 'Webhook URL', type: 'url', placeholder: 'https://discord.com/api/webhooks/...' }],
  },
  {
    id: 'prusaslicer',
    name: 'PrusaSlicer',
    category: 'Slicers',
    icon: 'settings',
    color: '#6c757d',
    description: 'Direct upload via OctoPrint API',
    longDesc: 'PrusaSlicer can upload G-code directly to OpenPolyPrint using the OctoPrint protocol. In PrusaSlicer: Settings → Physical Printers → Add. Set the API URL to your OpenPolyPrint address (e.g. http://openpolyprint). Leave the API key blank. Uploads go to the printer selected in Settings → Slicer upload target.',
    url: 'https://www.prusa3d.com/page/prusaslicer_424/',
    urlLabel: 'Download PrusaSlicer',
    alwaysActive: true,
    fields: [],
  },
  {
    id: 'orcaslicer',
    name: 'OrcaSlicer',
    category: 'Slicers',
    icon: 'settings2',
    color: '#ffc107',
    description: 'Direct upload via OctoPrint API',
    longDesc: 'OrcaSlicer supports direct upload to OpenPolyPrint via the OctoPrint protocol. In OrcaSlicer: Settings → Physical Printers → Add. Set the API URL to your OpenPolyPrint address (e.g. http://openpolyprint). Leave the API key blank. Uploads go to the printer selected in Settings → Slicer upload target. For per-printer routing, use http://openpolyprint/api/files/{printer_name}/local as the upload path.',
    url: 'https://github.com/SoftFever/OrcaSlicer',
    urlLabel: 'Download OrcaSlicer',
    alwaysActive: true,
    fields: [],
  },
  {
    id: 'cura',
    name: 'Cura',
    category: 'Slicers',
    icon: 'sliders',
    color: '#1a6cf5',
    description: 'Upload via OctoPrint plugin',
    longDesc: 'UltiMaker Cura can upload to OpenPolyPrint using the OctoPrint plugin. Install the OctoPrint plugin from Cura Marketplace, then add a printer with the OpenPolyPrint address as the OctoPrint URL. Uploads go to the printer selected in Settings → Slicer upload target.',
    url: 'https://ultimaker.com/software/ultimaker-cura/',
    urlLabel: 'Download Cura',
    alwaysActive: true,
    fields: [],
  },
  {
    id: 'n8n',
    name: 'n8n / Zapier',
    category: 'Automation',
    icon: 'zap',
    color: '#ea4b71',
    description: 'Automate with webhooks + REST API',
    longDesc: 'Use n8n or Zapier to automate workflows based on print events. Trigger webhooks on print start/complete/fail, or poll the OpenPolyPrint REST API for status.',
    url: 'https://n8n.io',
    urlLabel: 'n8n.io',
    alwaysActive: false,
    fields: [
      { id: 'webhook_url', label: 'Webhook URL', type: 'url', placeholder: 'https://n8n.example.com/webhook/print' },
      { id: 'events', label: 'Events (comma-separated)', type: 'text', placeholder: 'start,complete,fail' },
    ],
  },
]

export function testIntegration(id: string, config: Record<string, string> = {}, message = 'OpenPolyPrint integration test') {
  return fetch(`/api/integrations/${id}/test`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ config, message }),
  })
}
