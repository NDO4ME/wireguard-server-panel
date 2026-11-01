import { useEffect, useRef } from 'react'
import type { FormEvent } from 'react'
import { Icon } from './Icon'
import type { LoginPageConfig } from '../api/client'

interface LoginCardProps {
  cfg: LoginPageConfig
  preview?: boolean
  password?: string
  onPassword?: (v: string) => void
  onSubmit?: (e: FormEvent) => void
  error?: string
  loading?: boolean
}

// The sign-in card. It carries no brand of its own: only what the admin
// configured (logo, heading, accent colour) is shown.
export default function LoginCard({ cfg, preview, password = '', onPassword, onSubmit, error, loading }: LoginCardProps) {
  const ref = useRef<HTMLDivElement>(null)

  // Accent colour through the CSSOM (inline style attributes are blocked by the CSP).
  useEffect(() => {
    const el = ref.current
    if (!el) return
    const props = ['--primary', '--primary-hover', '--primary-press']
    if (/^#[0-9a-f]{6}$/i.test(cfg.accent)) {
      el.style.setProperty('--primary', cfg.accent)
      el.style.setProperty('--primary-hover', `color-mix(in srgb, ${cfg.accent} 85%, white)`)
      el.style.setProperty('--primary-press', `color-mix(in srgb, ${cfg.accent} 82%, black)`)
    } else {
      props.forEach(p => el.style.removeProperty(p))
    }
  }, [cfg.accent])

  return (
    <div className="auth-card" ref={ref} aria-hidden={preview || undefined}>
      {cfg.logo && (
        <div className="auth-logo">
          <img src={cfg.logo} alt="" draggable={false} />
        </div>
      )}
      {cfg.heading && <h1>{cfg.heading}</h1>}

      {error && !preview && (
        <div className="msg msg-err" role="alert">
          <Icon name="x-circle" />
          <div className="msg-body">{error}</div>
        </div>
      )}

      <form onSubmit={onSubmit} noValidate={preview}>
        <label className="field">
          <span className="sr-only">Password</span>
          <input
            type="password"
            value={password}
            onChange={e => onPassword?.(e.target.value)}
            placeholder="Password"
            autoComplete="current-password"
            autoFocus={!preview}
            required={!preview}
            readOnly={preview}
            tabIndex={preview ? -1 : undefined}
            spellCheck={false}
          />
        </label>
        <button
          type="submit"
          className={loading ? 'btn btn-primary btn-block is-busy' : 'btn btn-primary btn-block'}
          disabled={loading}
          tabIndex={preview ? -1 : undefined}
        >
          Sign in
        </button>
      </form>
    </div>
  )
}
