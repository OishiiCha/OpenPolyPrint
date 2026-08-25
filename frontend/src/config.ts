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

// loadConfigWithEnv fetches the backend config (which includes env-based
// secrets) and merges it with the localStorage config. Env-based values
// from the backend take precedence for secrets (geminiApiKey, etc.).
// Falls back to localStorage-only if the backend is unreachable.
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
    // Env-based values from backend override localStorage
    return {
      ...local,
      ...remote,
      // Only override if the env value is non-empty (otherwise keep local)
      geminiApiKey: remote.geminiApiKey || local.geminiApiKey,
      geminiEnabled: remote.geminiEnabled ?? local.geminiEnabled,
    } as AppConfig
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
  }).catch(() => {
    // backend not running; localStorage is the source of truth
  })
}
