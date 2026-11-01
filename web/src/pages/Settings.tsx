import { useState, useEffect } from 'react'
import { api, Settings as SettingsType } from '../api/client'
import Layout from '../components/Layout'
import { Icon } from '../components/Icon'

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

  const load = async () => {
    try {
      const data = await api.getSettings()
      setSettings(data)
      setDns(data.dns)
      setAllowedIPs(data.allowed_ips)
      setLoggingEnabled(data.logging_enabled)
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
    (dns !== settings.dns || allowedIPs !== settings.allowed_ips || loggingEnabled !== settings.logging_enabled || newPassword !== '')

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

    const update: { dns?: string; allowed_ips?: string; logging_enabled?: boolean; admin_password?: string } = {}
    if (dns !== settings?.dns) update.dns = dns
    if (allowedIPs !== settings?.allowed_ips) update.allowed_ips = allowedIPs
    if (loggingEnabled !== settings?.logging_enabled) update.logging_enabled = loggingEnabled
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
