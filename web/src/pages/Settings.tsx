import { useState, useEffect, useRef } from 'react'
import { api } from '../api/client'
import type { Settings as SettingsType, UpdateSettingsRequest } from '../api/client'
import Layout from '../components/Layout'
import { Icon } from '../components/Icon'
import LoginCard from '../components/LoginCard'

export default function Settings({ onLogout }: { onLogout: () => void }) {
  const [settings, setSettings] = useState<SettingsType | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState<{ kind: 'ok' | 'err'; text: string } | null>(null)

  const [dns, setDns] = useState('')
  const [allowedIPs, setAllowedIPs] = useState('')
  const [loggingEnabled, setLoggingEnabled] = useState(false)
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [loginTitle, setLoginTitle] = useState('')
  const [loginHeading, setLoginHeading] = useState('')
  const [loginLogo, setLoginLogo] = useState('')
  const [loginAccent, setLoginAccent] = useState('')
  const fileRef = useRef<HTMLInputElement>(null)

  const load = async () => {
    try {
      const data = await api.getSettings()
      setSettings(data)
      setDns(data.dns)
      setAllowedIPs(data.allowed_ips)
      setLoggingEnabled(data.logging_enabled)
      setLoginTitle(data.login_title)
      setLoginHeading(data.login_heading)
      setLoginLogo(data.login_logo)
      setLoginAccent(data.login_accent)
    } catch (err) {
      console.error('Failed to load settings:', err)
      setMessage({ kind: 'err', text: 'Failed to load settings.' })
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [])

  const dirty =
    !!settings &&
    (dns !== settings.dns || allowedIPs !== settings.allowed_ips || loggingEnabled !== settings.logging_enabled || newPassword !== '' ||
      loginTitle !== settings.login_title || loginHeading !== settings.login_heading || loginLogo !== settings.login_logo || loginAccent !== settings.login_accent)

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault()
    setMessage(null)

    if (newPassword && newPassword !== confirmPassword) {
      setMessage({ kind: 'err', text: 'Passwords do not match.' })
      return
    }
    if (newPassword && newPassword.length < 6) {
      setMessage({ kind: 'err', text: 'Password must be at least 6 characters.' })
      return
    }

    const update: UpdateSettingsRequest = {}
    if (dns !== settings?.dns) update.dns = dns
    if (allowedIPs !== settings?.allowed_ips) update.allowed_ips = allowedIPs
    if (loggingEnabled !== settings?.logging_enabled) update.logging_enabled = loggingEnabled
    if (loginTitle !== settings?.login_title) update.login_title = loginTitle
    if (loginHeading !== settings?.login_heading) update.login_heading = loginHeading
    if (loginLogo !== settings?.login_logo) update.login_logo = loginLogo
    if (loginAccent !== settings?.login_accent) update.login_accent = loginAccent
    if (newPassword) update.admin_password = newPassword
    if (Object.keys(update).length === 0) return

    setSaving(true)
    try {
      await api.updateSettings(update)
      setMessage({
        kind: 'ok',
        text: newPassword
          ? 'Settings saved. Sign in again with the new password to use the new API token.'
          : 'Settings saved. New client configs use these defaults.',
      })
      setNewPassword('')
      setConfirmPassword('')
      await load()
    } catch (err) {
      console.error('Failed to save settings:', err)
      setMessage({ kind: 'err', text: 'Failed to save settings.' })
    } finally {
      setSaving(false)
    }
  }

  const pickLogo = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (!file) return
    if (!['image/png', 'image/jpeg', 'image/webp', 'image/gif', 'image/svg+xml'].includes(file.type)) {
      setMessage({ kind: 'err', text: 'Logo must be a PNG, JPEG, WebP, GIF or SVG image.' })
      return
    }
    if (file.size > 256 * 1024) {
      setMessage({ kind: 'err', text: 'Logo must be smaller than 256 KB.' })
      return
    }
    const reader = new FileReader()
    reader.onload = () => setLoginLogo(String(reader.result))
    reader.readAsDataURL(file)
  }

  return (
    <Layout onLogout={onLogout}>
      {message && (
        <div className={`msg msg-${message.kind}`} role={message.kind === 'err' ? 'alert' : 'status'}>
          <Icon name={message.kind === 'ok' ? 'check-circle' : 'x-circle'} />
          <div className="msg-body">{message.text}</div>
          <button type="button" className="btn btn-ghost btn-icon btn-sm msg-close" aria-label="Dismiss" onClick={() => setMessage(null)}>
            <Icon name="x" />
          </button>
        </div>
      )}

      {loading ? (
        <div className="empty-state"><Icon name="refresh" />Loading settings…</div>
      ) : (
        <form onSubmit={handleSave}>
          <div className="cols cols-2">
            <section className="panel">
              <header className="panel-h"><h2><Icon name="globe" />Client defaults</h2></header>
              <div className="panel-b">
                <label className="field">
                  <span className="lbl">DNS servers</span>
                  <input type="text" value={dns} onChange={e => setDns(e.target.value)} placeholder="1.1.1.1, 8.8.8.8" />
                  <span className="hint">Written into every client config. Comma separated. Leave empty to omit the DNS line so clients keep their own resolver.</span>
                </label>
                <label className="field">
                  <span className="lbl">Allowed IPs</span>
                  <input type="text" value={allowedIPs} onChange={e => setAllowedIPs(e.target.value)} placeholder="0.0.0.0/0, ::/0" />
                  <span className="hint">Traffic routed through the VPN. <code>0.0.0.0/0, ::/0</code> sends everything; narrow it for split tunnelling.</span>
                </label>
                <label className="check">
                  <input type="checkbox" checked={loggingEnabled} onChange={e => setLoggingEnabled(e.target.checked)} />
                  <span>
                    Connection logging
                    <span className="hint">Record client endpoints (IP:port) for security audits and CGNAT identification.</span>
                  </span>
                </label>
              </div>
            </section>

            <section className="panel">
              <header className="panel-h"><h2><Icon name="lock" />Admin password</h2></header>
              <div className="panel-b">
                <label className="field">
                  <span className="lbl">New password <span className="opt">optional</span></span>
                  <input type="password" value={newPassword} onChange={e => setNewPassword(e.target.value)} autoComplete="new-password" placeholder="Leave empty to keep the current password" />
                </label>
                <label className="field">
                  <span className="lbl">Confirm password</span>
                  <input type="password" value={confirmPassword} onChange={e => setConfirmPassword(e.target.value)} autoComplete="new-password" />
                  <span className="hint">Changing the password also changes the API token.</span>
                </label>
              </div>
            </section>
          </div>

          <section className="panel">
            <header className="panel-h"><h2><Icon name="eye" />Login page</h2></header>
            <div className="panel-b login-editor">
              <div>
                <p className="hint hint-top">The sign-in page shows only a password field and a button. Everything below is optional and carries no product name.</p>
                <label className="field">
                  <span className="lbl">Logo <span className="opt">optional</span></span>
                  <div className="logo-pick">
                    <input ref={fileRef} type="file" accept="image/png,image/jpeg,image/webp,image/gif,image/svg+xml" onChange={pickLogo} className="sr-only" tabIndex={-1} />
                    <button type="button" className="btn btn-sm" onClick={() => fileRef.current?.click()}>
                      <Icon name="upload" />{loginLogo ? 'Replace logo' : 'Upload logo'}
                    </button>
                    {loginLogo && (
                      <button type="button" className="btn btn-ghost btn-sm" onClick={() => setLoginLogo('')}>
                        <Icon name="trash" />Remove
                      </button>
                    )}
                  </div>
                  <span className="hint">PNG, JPEG, WebP, GIF or SVG up to 256 KB. Also used as the tab icon on the sign-in page.</span>
                </label>
                <label className="field">
                  <span className="lbl">Heading <span className="opt">optional</span></span>
                  <input type="text" value={loginHeading} maxLength={60} onChange={e => setLoginHeading(e.target.value)} placeholder="No heading" />
                </label>
                <label className="field">
                  <span className="lbl">Browser tab title</span>
                  <input type="text" value={loginTitle} maxLength={60} onChange={e => setLoginTitle(e.target.value)} placeholder="Sign in" />
                </label>
                <label className="field">
                  <span className="lbl">Accent colour</span>
                  <div className="accent-pick">
                    <input type="color" value={/^#[0-9a-f]{6}$/i.test(loginAccent) ? loginAccent : '#2a8fdc'} onChange={e => setLoginAccent(e.target.value)} aria-label="Accent colour" />
                    <input type="text" value={loginAccent} onChange={e => setLoginAccent(e.target.value.trim())} placeholder="#2a8fdc" maxLength={7} spellCheck={false} />
                    {loginAccent && <button type="button" className="btn btn-ghost btn-sm" onClick={() => setLoginAccent('')}>Reset</button>}
                  </div>
                </label>
              </div>
              <div className="login-preview" aria-label="Login page preview">
                <LoginCard cfg={{ title: loginTitle, heading: loginHeading, logo: loginLogo, accent: loginAccent }} preview />
              </div>
            </div>
          </section>

          <div className="submit-bar">
            <button type="submit" className={saving ? 'btn btn-primary is-busy' : 'btn btn-primary'} disabled={saving || !dirty}>
              <Icon name="check" />
              Save changes
            </button>
            {dirty && <span className="dim">You have unsaved changes.</span>}
          </div>
        </form>
      )}
    </Layout>
  )
}
