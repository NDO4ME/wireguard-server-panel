import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { useState, useEffect, type ReactElement } from 'react'
import { api } from './api/client'
import { IconSprite } from './components/Icon'
import Login from './pages/Login'
import Clients from './pages/Clients'
import Settings from './pages/Settings'
import TailscalePage from './pages/Tailscale'
import SystemPage from './pages/System'
import ApiAccess from './pages/ApiAccess'

function App() {
  const [isAuthenticated, setIsAuthenticated] = useState<boolean | null>(null)

  useEffect(() => {
    const checkAuth = async () => {
      if (api.isAuthenticated()) {
        setIsAuthenticated(true)
      } else {
        setIsAuthenticated(await api.refresh())
      }
    }
    checkAuth()
  }, [])

  if (isAuthenticated === null) {
    return (
      <div className="boot" role="status" aria-label="Loading">
        <span className="boot-spin" />
      </div>
    )
  }

  const logout = () => setIsAuthenticated(false)
  const guard = (page: ReactElement) => (isAuthenticated ? page : <Navigate to="/login" replace />)

  return (
    <BrowserRouter>
      <IconSprite />
      <Routes>
        <Route
          path="/login"
          element={isAuthenticated ? <Navigate to="/" replace /> : <Login onLogin={() => setIsAuthenticated(true)} />}
        />
        <Route path="/" element={guard(<Clients onLogout={logout} />)} />
        <Route path="/tailscale" element={guard(<TailscalePage onLogout={logout} />)} />
        <Route path="/system" element={guard(<SystemPage onLogout={logout} />)} />
        <Route path="/api-access" element={guard(<ApiAccess onLogout={logout} />)} />
        <Route path="/settings" element={guard(<Settings onLogout={logout} />)} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  )
}

export default App
