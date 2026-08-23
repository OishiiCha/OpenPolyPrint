import type { Camera, GCodeFile, PrintRecord, Printer } from '../types'

export const isTest = new URLSearchParams(window.location.search).get('test') === '1'

export const mockPrinter: Printer = {
  id: 'test',
  name: 'Test Printer',
  type: 'anker-m5',
  state: 'printing',
  status: 'Printing',
  temps: {
    nozzle: 210,
    bed: 60,
    targetNozzle: 210,
    targetBed: 60,
  },
  progress: 42,
  currentFile: 'test_benchy.gcode',
  remainingTime: '1h 23m',
}

export const mockCamera: Camera = {
  id: 'c-test',
  name: 'Test Camera',
  printerId: 'test',
  type: 'stream',
  enabled: true,
  url: '',
  brightness: 0,
  flip: '',
}

export const mockGCodeFiles: GCodeFile[] = [
  { id: 'g1', name: 'Case_lid_v2.gcode', size: '12.4 MB', estimatedTime: '3h 20m', filament: '38.2 g' },
  { id: 'g2', name: 'Bracket_PLA.gcode', size: '4.1 MB', estimatedTime: '1h 05m', filament: '14.8 g' },
  { id: 'g3', name: 'Vase_mode_v3.gcode', size: '21.8 MB', estimatedTime: '5h 15m', filament: '92.5 g' },
]

export const mockHistory: PrintRecord[] = [
  { id: 'h1', printer: 'Anker M5', file: 'Case_lid_v2.gcode', started: '2026-08-20 14:30', duration: '3h 18m', result: 'Success' },
  { id: 'h2', printer: 'Voron Trident', file: 'Bracket_PLA.gcode', started: '2026-08-19 09:15', duration: '1h 12m', result: 'Failed' },
]
