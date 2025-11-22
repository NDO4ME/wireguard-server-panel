import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { useState, useEffect } from 'react'
import { api } from './api/client'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import Settings from './pages/Settings'

function App() {
  const [isAuthenticated, setIsAuthenticated] = useState<boolean | null>(null)
  const [showSettings, setShowSettings] = useState(false)

  useEffect(() => {
    // Check if we have a valid stored token first
    const checkAuth = async () => {
      if (api.isAuthenticated()) {
        // Token exists and is valid
        setIsAuthenticated(true)
      } else {
        // Try to refresh token
        const success = await api.refresh()
        setIsAuthenticated(success)
      }
    }
    checkAuth()
  }, [])

  if (isAuthenticated === null) {
    return (
      <div style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        height: '100vh'
      }}>
        <div className="animate-spin" style={{
          width: 40,
          height: 40,
          border: '3px solid var(--bg-tertiary)',
          borderTopColor: 'var(--accent)',
          borderRadius: '50%'
        }} />
      </div>
    )
  }

  return (
    <BrowserRouter>
      <Routes>
        <Route
          path="/login"
          element={
            isAuthenticated
              ? <Navigate to="/" replace />
              : <Login onLogin={() => setIsAuthenticated(true)} />
          }
        />
        <Route
          path="/"
          element={
            isAuthenticated
              ? (showSettings
                  ? <Settings onBack={() => setShowSettings(false)} />
                  : <Dashboard
                      onLogout={() => setIsAuthenticated(false)}
                      onSettings={() => setShowSettings(true)}
                    />
                )
              : <Navigate to="/login" replace />
          }
        />
      </Routes>
    </BrowserRouter>
  )
}

export default App
