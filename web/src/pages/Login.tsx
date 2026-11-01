import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { api } from '../api/client'
import type { LoginPageConfig } from '../api/client'
import LoginCard from '../components/LoginCard'

const EMPTY: LoginPageConfig = { title: '', heading: '', logo: '', accent: '' }

export default function Login({ onLogin }: { onLogin: () => void }) {
  const [cfg, setCfg] = useState<LoginPageConfig | null>(null)
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    let alive = true
    api.getLoginPage().then(c => alive && setCfg(c)).catch(() => alive && setCfg(EMPTY))
    return () => { alive = false }
  }, [])

  // Neutral tab title; a configured logo doubles as the favicon.
  useEffect(() => {
    if (!cfg) return
    const prevTitle = document.title
    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    const prevHref = link?.getAttribute('href') ?? null
    document.title = cfg.title || 'Sign in'
    if (link && cfg.logo) {
      link.removeAttribute('type')
      link.setAttribute('href', cfg.logo)
    }
    return () => {
      document.title = prevTitle
      if (link && prevHref !== null) link.setAttribute('href', prevHref)
    }
  }, [cfg])

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      // The admin username is fixed and cannot be changed
      if (await api.login('ndo4me', password)) {
        onLogin()
      } else {
        setError('Invalid password.')
      }
    } catch {
      setError('Connection error. Please try again.')
    } finally {
      setLoading(false)
    }
  }

  if (!cfg) return <div className="boot" role="status" aria-label="Loading"><div className="boot-spin" /></div>

  return (
    <main className="auth">
      <div className="auth-wrap">
        <LoginCard cfg={cfg} password={password} onPassword={setPassword} onSubmit={handleSubmit} error={error} loading={loading} />
      </div>
    </main>
  )
}
