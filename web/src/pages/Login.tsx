import { useState } from 'react'
import { Icon } from '../components/Icon'
import { api } from '../api/client'

interface LoginProps {
  onLogin: () => void
}

export default function Login({ onLogin }: LoginProps) {
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)

    try {
      // The admin username is fixed and cannot be changed
      const success = await api.login('ndo4me', password)
      if (success) {
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

  return (
    <main className="auth">
      <div className="auth-wrap">
        <div className="auth-card">
          <div className="auth-brand">
            <span className="brand-lockup">
              <Icon name="brand" className="brand-mark brand-mark-lg" />
              <span className="brand-word">NDO4ME</span>
            </span>
          </div>
          <h1>Sign in</h1>
          <p className="ph-desc">Enter the admin password for this WireGuard server.</p>

          {error && (
            <div className="msg msg-err" role="alert">
              <Icon name="x-circle" />
              <div className="msg-body">{error}</div>
            </div>
          )}

          <form onSubmit={handleSubmit}>
            <label className="field">
              <span className="lbl">Password</span>
              <input
                type="password"
                value={password}
                onChange={e => setPassword(e.target.value)}
                autoComplete="current-password"
                autoFocus
                required
                spellCheck={false}
              />
            </label>
            <button type="submit" className={loading ? 'btn btn-primary btn-block is-busy' : 'btn btn-primary btn-block'} disabled={loading}>
              Sign in
            </button>
          </form>
          <div className="auth-foot">
            Forgot the password? Set <code>ADMIN_PASSWORD</code> and recreate the database as described in the README.
          </div>
        </div>
        <div className="auth-meta">
          <Icon name="lock" /> Use HTTPS in production · repeated failures are rate-limited
        </div>
      </div>
    </main>
  )
}
