export interface ProviderConfig {
  enabled: boolean
  username: string
  password: string
  apiKey: string
  host: string
}

export interface IntegrationConfig {
  enabled: boolean
  fields: Record<string, string>
}

import { integrations } from './integrations'

import type { AutoRecordSettings, TimelapseSettings } from './types'

export interface AppConfig {
  dark: boolean
  compact: boolean
  showMiniTerminal: boolean
  slicerTarget: string
  geminiApiKey: string
  geminiEnabled: boolean
  analyticsEnabled: boolean
  authPasscode: string
  notifyFinished: boolean
  notifyFailed: boolean
  providers: {
    anker: ProviderConfig
    flashforge: ProviderConfig
    klipper: ProviderConfig
    other: ProviderConfig
  }
  integrations: Record<string, IntegrationConfig>
  timelapse: TimelapseSettings
  autoRecord: AutoRecordSettings
}

const STORAGE_KEY = 'openpolyprint-config'

const alwaysActiveIntegrations: Record<string, IntegrationConfig> = Object.fromEntries(
  integrations.filter((i) => i.alwaysActive).map((i) => [i.id, { enabled: true, fields: {} }])
)

const defaultProvider = (): ProviderConfig => ({
  enabled: false,
  username: '',
  password: '',
  apiKey: '',
  host: '',
})

const defaultTimelapse = (): TimelapseSettings => ({
  enabled: false,
  rate: '1fps',
})

const defaultAutoRecord = (): AutoRecordSettings => ({
  enabled: false,
  mode: 'video',
  interval: 5,
})

export function defaultConfig(): AppConfig {
  return {
    dark: window.matchMedia('(prefers-color-scheme: dark)').matches,
    compact: false,
    showMiniTerminal: false,
    slicerTarget: '',
    geminiApiKey: '',
    geminiEnabled: false,
    analyticsEnabled: true,
    authPasscode: '',
    notifyFinished: true,
    notifyFailed: true,
    providers: {
      anker: defaultProvider(),
      flashforge: defaultProvider(),
      klipper: defaultProvider(),
      other: defaultProvider(),
    },
    integrations: alwaysActiveIntegrations,
    timelapse: defaultTimelapse(),
    autoRecord: defaultAutoRecord(),
  }
}

export function loadConfig(): AppConfig {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const parsed = JSON.parse(raw) as Partial<AppConfig>
      const fallback = defaultConfig()
      return {
        ...fallback,
        ...parsed,
        providers: {
          anker: { ...fallback.providers.anker, ...parsed.providers?.anker },
          flashforge: { ...fallback.providers.flashforge, ...parsed.providers?.flashforge },
          klipper: { ...fallback.providers.klipper, ...parsed.providers?.klipper },
          other: { ...fallback.providers.other, ...parsed.providers?.other },
        },
        integrations: { ...alwaysActiveIntegrations, ...fallback.integrations, ...parsed.integrations },
        timelapse: { ...fallback.timelapse, ...parsed.timelapse },
        autoRecord: { ...fallback.autoRecord, ...parsed.autoRecord },
      }
    }
  } catch (e) {
    // ignore corrupt localStorage
  }
  return defaultConfig()
}

// loadConfigWithEnv fetches env-based secrets from the backend (e.g.
// GEMINI_API_KEY from .env) and merges them with the localStorage config.
// Only env-derived values override localStorage — UI-toggled settings like
// geminiEnabled, dark mode, etc. always come from localStorage (the source
// of truth for user-editable preferences).
export async function loadConfigWithEnv(): Promise<AppConfig> {
  const local = loadConfig()
  try {
    const res = await fetch('/api/config')
    if (!res.ok) return local
    const remote = await res.json() as Partial<AppConfig> & {
      geminiApiKey?: string
      geminiEnabled?: boolean
      envAnkerEmail?: string
      envAnkerRegion?: string
    }
    // Only take env-only values from the backend. localStorage is the
    // source of truth for all UI-toggled settings.
    return {
      ...local,
      // Env-based secrets (from .env file via backend)
      geminiApiKey: remote.geminiApiKey || local.geminiApiKey,
      // geminiEnabled from env overrides localStorage; otherwise keep local.
      // The backend only includes geminiEnabled when GEMINI_ENABLED env var
      // is explicitly set — otherwise it's absent and we keep the local value.
      geminiEnabled: remote.geminiEnabled !== undefined ? remote.geminiEnabled : local.geminiEnabled,
    }
  } catch {
    return local
  }
}

export function saveConfig(config: AppConfig): void {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(config))
  window.dispatchEvent(new CustomEvent('openpolyprint-config-updated'))
  fetch('/api/config', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(config),
  }).catch((err) => {
    console.error('[config] failed to save to backend:', err)
  })
}
