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

  if (isPhone) {
    return (
      <BrowserRouter>
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
      <Routes>
        <Route path="/" element={<Layout />}>
          <Route index element={<Dashboard />} />
          <Route path="printers" element={<Printers />} />
          <Route path="printers/:id" element={<PrinterDetail />} />
          <Route path="printers/:id/leveling" element={<div className="p-8 text-center">Bed Leveling</div>} />
          <Route path="gcode" element={<GCode />} />
          <Route path="gcode/:id" element={<GCodeDetail />} />
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
