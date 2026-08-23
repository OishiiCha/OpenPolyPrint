export type PrinterType = 'anker-m5' | 'anker-m5c' | 'flashforge' | 'klipper' | string

export interface Printer {
  id: string
  name: string
  type: string
  state?: string
  status: 'Idle' | 'Printing' | 'Paused' | 'Offline' | 'Heating' | string
  temps: {
    nozzle: number
    bed: number
    targetNozzle: number
    targetBed: number
  }
  progress: number
  currentFile?: string
  remainingTime?: string
}

export interface GCodeFile {
  id: string
  name: string
  size: string
  fileSize?: number
  estimatedTime?: string
  filament?: string
  thumbnail?: string
  printerId?: string
}

export interface PrintRecord {
  id: string
  printer: string
  file: string
  started: string
  duration: string
  result: 'Success' | 'Failed' | 'Cancelled'
}

export type CameraType = 'built-in' | 'usb' | 'stream' | 'mipi'

export interface Camera {
  id: string
  name: string
  printerId: string
  type: CameraType
  enabled: boolean
  url?: string
  deviceId?: string
  deviceLabel?: string
  sensor?: string
  brightness?: number
  flip?: string
}

export type TimelapseRate = '1fps' | '5fps' | '10fps' | '30fpm' | '10fpm' | '1fpm'

export interface TimelapseSettings {
  enabled: boolean
  rate: TimelapseRate
}

export interface AutoRecordSettings {
  enabled: boolean
  mode: 'video' | 'timelapse'
  interval: number
}
