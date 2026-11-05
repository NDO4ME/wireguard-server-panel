import { useState, useEffect } from 'react'
import { api } from '../api/client'
import Layout from '../components/Layout'
import Modal from '../components/Modal'
import { Icon } from '../components/Icon'

const ENDPOINTS: [string, string, string][] = [
  ['GET', '/api/v1/peers', 'List clients'],
  ['POST', '/api/v1/peers', 'Create a client  {"name": "Phone"}'],
  ['PATCH', '/api/v1/peers/:ip', 'Rename or enable/disable  {"name"?, "enabled"?}'],
  ['DELETE', '/api/v1/peers/:ip', 'Delete a client'],
  ['GET', '/api/v1/peers/:ip/config', 'Download the .conf file'],
  ['GET', '/api/v1/peers/:ip/qrcode', 'QR code (PNG)'],
  ['GET', '/api/v1/peers/:ip/logs', 'Connection log'],
  ['GET', '/api/v1/settings', 'Read settings'],
  ['PUT', '/api/v1/settings', 'Update settings'],
  ['GET', '/api/v1/tailscale/status', 'Tailscale status'],
  ['GET', '/api/v1/tailscale/prefs', 'Exit node, routes, shields-up'],
  ['GET', '/api/v1/system/status', 'Versions, interface and client counts'],
  ['GET', '/api/v1/settings/backup', 'Download a database backup'],
  ['GET', '/api/health', 'Health check (no auth)'],
]

export default function ApiAccess({ onLogout }: { onLogout: () => void }) {
  const [token, setToken] = useState('')
  const [loading, setLoading] = useState(true)
  const [copied, setCopied] = useState<string | null>(null)
  const [confirmRotate, setConfirmRotate] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  useEffect(() => {
    api.getSettings().then(s => setToken(s.api_token || '')).catch(() => {}).finally(() => setLoading(false))
  }, [])

  const copy = async (what: string, text: string) => {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(what)
      setTimeout(() => setCopied(null), 1800)
    } catch {
      /* clipboard unavailable (non-HTTPS) */
    }
  }

  const rotate = async () => {
    setConfirmRotate(false)
    setErr(null)
    try {
      setToken(await api.rotateAPIToken())
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Failed to rotate token')
    }
  }

  const origin = window.location.origin
  const example = `curl -H "Authorization: Bearer ${token || 'YOUR_TOKEN'}" \\\n  ${origin}/api/v1/peers`

  return (
    <Layout onLogout={onLogout}>
      <div className="cols cols-main">
        <section className="panel">
          <header className="panel-h"><h2><Icon name="code" />Endpoints</h2></header>
          <div className="tbl-wrap">
            <table className="tbl tbl-compact">
              <thead><tr><th>Method</th><th>Path</th><th>Description</th></tr></thead>
              <tbody>
                {ENDPOINTS.map(([m, p, d]) => (
                  <tr key={m + p}>
                    <td><span className={`badge ${m === 'DELETE' ? 'failed' : m === 'GET' ? 'info' : 'warning'}`}>{m}</span></td>
                    <td className="mono">{p}</td>
                    <td>{d}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>

        <div>
          <section className="panel">
            <header className="panel-h"><h2><Icon name="key" />API token</h2></header>
            <div className="panel-b">
              {loading ? (
                <span className="dim">Loading…</span>
              ) : token ? (
                <div className="secret-v">
                  <code>{token}</code>
                  <button type="button" className="btn btn-sm" onClick={() => copy('token', token)}>
                    <Icon name={copied === 'token' ? 'check' : 'copy'} />
                    {copied === 'token' ? 'Copied' : 'Copy'}
                  </button>
                </div>
              ) : (
                <div className="msg msg-warn compact">
                  <Icon name="alert" />
                  <div className="msg-body">No token yet. Sign out and sign in again to generate it.</div>
                </div>
              )}
              {err && <div className="msg msg-err compact" role="alert"><Icon name="x-circle" /><div className="msg-body">{err}</div></div>}
              <span className="hint">Random and independent of the password; regenerated when the password changes. Send it as a Bearer token.</span>
              <div className="form-actions">
                <button type="button" className="btn btn-sm" onClick={() => setConfirmRotate(true)}><Icon name="refresh" />Rotate token</button>
              </div>
            </div>
          </section>

          <section className="panel">
            <header className="panel-h">
              <h2><Icon name="terminal" />Example</h2>
              <div className="panel-tools">
                <button type="button" className="btn btn-ghost btn-sm" onClick={() => copy('example', example)}>
                  <Icon name={copied === 'example' ? 'check' : 'copy'} />
                  {copied === 'example' ? 'Copied' : 'Copy'}
                </button>
              </div>
            </header>
            <div className="panel-b"><pre className="codebox code-pre">{example}</pre></div>
          </section>
        </div>
      </div>
      {confirmRotate && (
        <Modal
          title="Rotate API token"
          icon="key"
          danger
          onClose={() => setConfirmRotate(false)}
          footer={
            <>
              <button type="button" className="btn" onClick={() => setConfirmRotate(false)}>Cancel</button>
              <button type="button" className="btn btn-danger" onClick={rotate}>Rotate</button>
            </>
          }
        >
          <p>A new random token is generated and the current one stops working immediately. Scripts and integrations using it must be updated.</p>
        </Modal>
      )}
    </Layout>
  )
}
