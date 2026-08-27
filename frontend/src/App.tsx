import { useEffect, useState } from 'react'
import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { Layout } from './components/Layout'
import { PhoneLayout } from './components/PhoneLayout'
import {
  Dashboard,
  Printers,
  PrinterDetail,
  GCode,
  GCodeDetail,
  Cameras,
  Pi,
  History,
  Terminal,
  Settings,
  Help,
} from './pages'
import { Recordings } from './pages/Recordings'
import { BedLeveling } from './pages/BedLeveling'
import { PrintQueue } from './pages/PrintQueue'
import { Filament } from './pages/Filament'
import { SmartPlugs } from './pages/SmartPlugs'
import { PrintAnalysis } from './pages/PrintAnalysis'
import { Analytics } from './pages/Analytics'
import { Profiles } from './pages/Profiles'
import { Login } from './pages/Login'
import { BackgroundStreams } from './components/BackgroundStreams'
import { useAuth } from './hooks/useAuth'

function useIsPhone() {
  const [isPhone, setIsPhone] = useState(() => window.matchMedia('(max-width: 767px)').matches)
  useEffect(() => {
    const mq = window.matchMedia('(max-width: 767px)')
    const handler = (e: MediaQueryListEvent) => setIsPhone(e.matches)
    mq.addEventListener('change', handler)
    return () => mq.removeEventListener('change', handler)
  }, [])
  return isPhone
}

function App() {
  const isPhone = useIsPhone()
  const { enabled, authenticated, loading, login } = useAuth()

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-slate-50 dark:bg-slate-950">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-blue-500 border-t-transparent" />
      </div>
    )
  }

  // Show login screen if auth is enabled and user is not authenticated
  if (enabled && !authenticated) {
    return <Login onLogin={login} />
  }

  if (isPhone) {
    return (
      <BrowserRouter>
        <BackgroundStreams />
        <PhoneLayout>
          <Routes>
            <Route index element={<Dashboard />} />
          </Routes>
        </PhoneLayout>
      </BrowserRouter>
    )
  }

  return (
    <BrowserRouter>
      <BackgroundStreams />
      <Routes>
        <Route path="/" element={<Layout />}>
          <Route index element={<Dashboard />} />
          <Route path="printers" element={<Printers />} />
          <Route path="printers/:id" element={<PrinterDetail />} />
          <Route path="printers/:id/leveling" element={<BedLeveling />} />
          <Route path="gcode" element={<GCode />} />
          <Route path="gcode/:id" element={<GCodeDetail />} />
          <Route path="queue" element={<PrintQueue />} />
          <Route path="filament" element={<Filament />} />
          <Route path="profiles" element={<Profiles />} />
          <Route path="plugs" element={<SmartPlugs />} />
          <Route path="analysis" element={<PrintAnalysis />} />
          <Route path="analytics" element={<Analytics />} />
          <Route path="cameras" element={<Cameras />} />
          <Route path="recordings" element={<Recordings />} />
          <Route path="pi" element={<Pi />} />
          <Route path="history" element={<History />} />
          <Route path="terminal" element={<Terminal />} />
          <Route path="settings" element={<Settings />} />
          <Route path="help" element={<Help />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}

export default App
