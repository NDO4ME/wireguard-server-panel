import { useState, useEffect } from 'react'
import {
  ArrowLeft,
  Save,
  Globe,
  FileText,
  Lock,
  Loader2,
  Key,
  Copy,
  Check,
  ChevronDown,
  ChevronUp,
  Network,
  ExternalLink,
  Power,
  PowerOff,
  Router
} from 'lucide-react'
import { api, Settings as SettingsType, TailscaleStatus } from '../api/client'
import '../styles/settings.css'

interface SettingsProps {
  onBack: () => void
}

export default function Settings({ onBack }: SettingsProps) {
  const [settings, setSettings] = useState<SettingsType | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  // Form state
  const [dns, setDns] = useState('')
  const [allowedIPs, setAllowedIPs] = useState('')
  const [loggingEnabled, setLoggingEnabled] = useState(false)
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [tokenCopied, setTokenCopied] = useState(false)
  const [showApiDocs, setShowApiDocs] = useState(false)

  // Tailscale state
  const [tailscale, setTailscale] = useState<TailscaleStatus | null>(null)
  const [tailscaleLoading, setTailscaleLoading] = useState(false)
  const [tailscaleMessage, setTailscaleMessage] = useState<{ type: 'success' | 'error' | 'info'; text: string } | null>(null)

  useEffect(() => {
    fetchSettings()
    fetchTailscaleStatus()
  }, [])

  const fetchSettings = async () => {
    try {
      const data = await api.getSettings()
      setSettings(data)
      setDns(data.dns)
      setAllowedIPs(data.allowed_ips)
      setLoggingEnabled(data.logging_enabled)
    } catch (err) {
      console.error('Failed to load settings:', err)
      setMessage({ type: 'error', text: 'Failed to load settings' })
    } finally {
      setLoading(false)
    }
  }

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault()

    // Validate password if provided
    if (newPassword && newPassword !== confirmPassword) {
      setMessage({ type: 'error', text: 'Passwords do not match' })
      return
    }

    if (newPassword && newPassword.length < 6) {
      setMessage({ type: 'error', text: 'Password must be at least 6 characters' })
      return
    }

    setSaving(true)
    setMessage(null)

    try {
      const updateData: { dns?: string; allowed_ips?: string; logging_enabled?: boolean; admin_password?: string } = {}

      if (dns !== settings?.dns) {
        updateData.dns = dns
      }

      if (allowedIPs !== settings?.allowed_ips) {
        updateData.allowed_ips = allowedIPs
      }

      if (loggingEnabled !== settings?.logging_enabled) {
        updateData.logging_enabled = loggingEnabled
      }

      if (newPassword) {
        updateData.admin_password = newPassword
      }

      if (Object.keys(updateData).length > 0) {
        await api.updateSettings(updateData)

        // If password changed, refetch settings to get new API token
        if (newPassword) {
          setMessage({ type: 'success', text: 'Settings saved. Please login again with your new password to use the new API token.' })
          await fetchSettings()
        } else {
          setMessage({ type: 'success', text: 'Settings saved successfully' })
          // Update local settings state
          setSettings(prev => prev ? {
            ...prev,
            dns: dns,
            allowed_ips: allowedIPs,
            logging_enabled: loggingEnabled
          } : null)
        }

        setNewPassword('')
        setConfirmPassword('')
      } else {
        setMessage({ type: 'success', text: 'No changes to save' })
      }
    } catch (err) {
      console.error('Failed to save settings:', err)
      setMessage({ type: 'error', text: 'Failed to save settings' })
    } finally {
      setSaving(false)
    }
  }

  const handleCopyToken = async () => {
    const token = settings?.api_token
    if (token) {
      try {
        await navigator.clipboard.writeText(token)
        setTokenCopied(true)
        setTimeout(() => setTokenCopied(false), 2000)
      } catch {
        // Fallback for older browsers
        const textArea = document.createElement('textarea')
        textArea.value = token
        document.body.appendChild(textArea)
        textArea.select()
        document.execCommand('copy')
        document.body.removeChild(textArea)
        setTokenCopied(true)
        setTimeout(() => setTokenCopied(false), 2000)
      }
    }
  }

  // Tailscale functions
  const fetchTailscaleStatus = async () => {
    try {
      const data = await api.getTailscaleStatus()
      setTailscale(data)
    } catch (err) {
      console.error('Failed to load Tailscale status:', err)
    }
  }

  const handleTailscaleConnect = async () => {
    setTailscaleLoading(true)
    setTailscaleMessage(null)
    try {
      const result = await api.connectTailscale()
      if (result.auth_url) {
        setTailscaleMessage({
          type: 'info',
          text: `Please authenticate: ${result.auth_url}`
        })
        // Open auth URL in new tab
        window.open(result.auth_url, '_blank')

        // Start polling for status changes every 3 seconds
        const pollInterval = setInterval(async () => {
          try {
            const status = await api.getTailscaleStatus()
            setTailscale(status)
            // Stop polling when connected
            if (status.connected) {
              clearInterval(pollInterval)
              setTailscaleMessage({ type: 'success', text: 'Connected to Tailscale' })
            }
          } catch (err) {
            console.error('Failed to poll Tailscale status:', err)
          }
        }, 3000)

        // Stop polling after 2 minutes
        setTimeout(() => clearInterval(pollInterval), 120000)
      } else {
        setTailscaleMessage({ type: 'success', text: result.message })
      }
      await fetchTailscaleStatus()
    } catch (err) {
      setTailscaleMessage({ type: 'error', text: err instanceof Error ? err.message : 'Failed to connect' })
    } finally {
      setTailscaleLoading(false)
    }
  }

  const handleTailscaleDisconnect = async () => {
    setTailscaleLoading(true)
    setTailscaleMessage(null)
    try {
      const result = await api.disconnectTailscale()
      setTailscaleMessage({ type: 'success', text: result.message })
      await fetchTailscaleStatus()
    } catch (err) {
      setTailscaleMessage({ type: 'error', text: err instanceof Error ? err.message : 'Failed to disconnect' })
    } finally {
      setTailscaleLoading(false)
    }
  }

  const handleEnableRouting = async () => {
    setTailscaleLoading(true)
    setTailscaleMessage(null)
    try {
      const result = await api.enableTailscaleRouting()
      setTailscaleMessage({ type: 'success', text: result.message })
      await fetchTailscaleStatus()
    } catch (err) {
      setTailscaleMessage({ type: 'error', text: err instanceof Error ? err.message : 'Failed to enable routing' })
    } finally {
      setTailscaleLoading(false)
    }
  }

  const handleDisableRouting = async () => {
    setTailscaleLoading(true)
    setTailscaleMessage(null)
    try {
      const result = await api.disableTailscaleRouting()
      setTailscaleMessage({ type: 'success', text: result.message })
      await fetchTailscaleStatus()
    } catch (err) {
      setTailscaleMessage({ type: 'error', text: err instanceof Error ? err.message : 'Failed to disable routing' })
    } finally {
      setTailscaleLoading(false)
    }
  }

  if (loading) {
    return (
      <div className="settings-page">
        <div className="loading-state">
          <Loader2 size={32} className="spin" />
          <p>Loading settings...</p>
        </div>
      </div>
    )
  }

  return (
    <div className="settings-page">
      <header className="settings-header">
        <button onClick={onBack} className="back-btn">
          <ArrowLeft size={20} />
          <span>Back to Dashboard</span>
        </button>
        <h1>Settings</h1>
      </header>

      <form onSubmit={handleSave} className="settings-form">
        {message && (
          <div className={`message ${message.type}`}>
            {message.text}
          </div>
        )}

        <section className="settings-section">
          <h2>
            <Globe size={18} />
            Network Settings
          </h2>
          <div className="form-group">
            <label htmlFor="dns">DNS Server</label>
            <input
              id="dns"
              type="text"
              value={dns}
              onChange={e => setDns(e.target.value)}
              placeholder="1.1.1.1"
            />
            <span className="hint">DNS server for VPN clients (e.g., 1.1.1.1, 8.8.8.8)</span>
          </div>
          <div className="form-group">
            <label htmlFor="allowedIPs">Allowed IPs</label>
            <input
              id="allowedIPs"
              type="text"
              value={allowedIPs}
              onChange={e => setAllowedIPs(e.target.value)}
              placeholder="0.0.0.0/0, ::/0"
            />
            <span className="hint">IP ranges to route through VPN (e.g., 0.0.0.0/0, ::/0 for all traffic)</span>
          </div>
        </section>

        <section className="settings-section">
          <h2>
            <FileText size={18} />
            Security Logging
          </h2>
          <div className="form-group">
            <label className="toggle-label">
              <span>Connection Logging</span>
              <div className="toggle-switch">
                <input
                  type="checkbox"
                  checked={loggingEnabled}
                  onChange={e => setLoggingEnabled(e.target.checked)}
                />
                <span className="toggle-slider"></span>
              </div>
            </label>
            <span className="hint">
              Log client connection endpoints (IP:Port) for security audit purposes.
              Useful for CGNAT identification and compliance requirements.
            </span>
          </div>
        </section>

        <section className="settings-section">
          <h2>
            <Lock size={18} />
            Admin Password
          </h2>
          <div className="form-group">
            <label htmlFor="newPassword">New Password</label>
            <input
              id="newPassword"
              type="password"
              value={newPassword}
              onChange={e => setNewPassword(e.target.value)}
              placeholder="Leave empty to keep current"
            />
          </div>
          <div className="form-group">
            <label htmlFor="confirmPassword">Confirm Password</label>
            <input
              id="confirmPassword"
              type="password"
              value={confirmPassword}
              onChange={e => setConfirmPassword(e.target.value)}
              placeholder="Confirm new password"
            />
          </div>
        </section>

        <section className="settings-section">
          <h2>
            <Key size={18} />
            API Token
          </h2>
          <div className="form-group">
            <label>Current Token</label>
            <div className="token-display">
              <code className="token-value">
                {settings?.api_token || 'Login again to generate token'}
              </code>
              <button
                type="button"
                onClick={handleCopyToken}
                className="copy-token-btn"
                title="Copy token"
                disabled={!settings?.api_token}
              >
                {tokenCopied ? <Check size={16} /> : <Copy size={16} />}
              </button>
            </div>
            <span className="hint">
              This token is derived from your password and stays constant. It will only change when you update your password.
            </span>
          </div>
          <div className="form-group">
            <button
              type="button"
              className="api-docs-toggle"
              onClick={() => setShowApiDocs(!showApiDocs)}
            >
              <span>API Documentation</span>
              {showApiDocs ? <ChevronUp size={16} /> : <ChevronDown size={16} />}
            </button>

            {showApiDocs && (
              <div className="api-docs">
                <div className="api-section">
                  <h4>Authentication</h4>
                  <p>All API requests require authentication via Bearer token in the header:</p>
                  <pre className="code-block">
{`curl -X GET "${window.location.origin}/api/v1/peers" \\
  -H "Authorization: Bearer YOUR_TOKEN" \\
  -H "Content-Type: application/json"`}
                  </pre>
                </div>

                <div className="api-section">
                  <h4>Endpoints</h4>
                  <div className="api-endpoints">
                    <table className="endpoints-table">
                      <thead>
                        <tr>
                          <th>Method</th>
                          <th>Endpoint</th>
                          <th>Description</th>
                        </tr>
                      </thead>
                      <tbody>
                        <tr>
                          <td><span className="method get">GET</span></td>
                          <td><code>/api/v1/peers</code></td>
                          <td>List all peers</td>
                        </tr>
                        <tr>
                          <td><span className="method post">POST</span></td>
                          <td><code>/api/v1/peers</code></td>
                          <td>Create new peer</td>
                        </tr>
                        <tr>
                          <td><span className="method patch">PATCH</span></td>
                          <td><code>/api/v1/peers/:ip</code></td>
                          <td>Update peer</td>
                        </tr>
                        <tr>
                          <td><span className="method delete">DELETE</span></td>
                          <td><code>/api/v1/peers/:ip</code></td>
                          <td>Delete peer</td>
                        </tr>
                        <tr>
                          <td><span className="method get">GET</span></td>
                          <td><code>/api/v1/peers/:ip/config</code></td>
                          <td>Download config file</td>
                        </tr>
                        <tr>
                          <td><span className="method get">GET</span></td>
                          <td><code>/api/v1/peers/:ip/qrcode</code></td>
                          <td>Get QR code image</td>
                        </tr>
                        <tr>
                          <td><span className="method get">GET</span></td>
                          <td><code>/api/v1/peers/:ip/logs</code></td>
                          <td>Get connection logs</td>
                        </tr>
                        <tr>
                          <td><span className="method get">GET</span></td>
                          <td><code>/api/v1/settings</code></td>
                          <td>Get settings</td>
                        </tr>
                        <tr>
                          <td><span className="method put">PUT</span></td>
                          <td><code>/api/v1/settings</code></td>
                          <td>Update settings</td>
                        </tr>
                      </tbody>
                    </table>
                  </div>
                </div>

                <div className="api-section">
                  <h4>Examples</h4>

                  <div className="api-example">
                    <span className="example-title">List all peers:</span>
                    <pre className="code-block">
{`curl -X GET "${window.location.origin}/api/v1/peers" \\
  -H "Authorization: Bearer YOUR_TOKEN"`}
                    </pre>
                  </div>

                  <div className="api-example">
                    <span className="example-title">Create new peer:</span>
                    <pre className="code-block">
{`curl -X POST "${window.location.origin}/api/v1/peers" \\
  -H "Authorization: Bearer YOUR_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{"name": "My Phone"}'`}
                    </pre>
                  </div>

                  <div className="api-example">
                    <span className="example-title">Update peer (enable/disable or rename):</span>
                    <pre className="code-block">
{`curl -X PATCH "${window.location.origin}/api/v1/peers/10.8.0.2" \\
  -H "Authorization: Bearer YOUR_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{"enabled": false, "name": "New Name"}'`}
                    </pre>
                  </div>

                  <div className="api-example">
                    <span className="example-title">Delete peer:</span>
                    <pre className="code-block">
{`curl -X DELETE "${window.location.origin}/api/v1/peers/10.8.0.2" \\
  -H "Authorization: Bearer YOUR_TOKEN"`}
                    </pre>
                  </div>

                  <div className="api-example">
                    <span className="example-title">Download config:</span>
                    <pre className="code-block">
{`curl -X GET "${window.location.origin}/api/v1/peers/10.8.0.2/config" \\
  -H "Authorization: Bearer YOUR_TOKEN" \\
  -o client.conf`}
                    </pre>
                  </div>

                  <div className="api-example">
                    <span className="example-title">Update settings:</span>
                    <pre className="code-block">
{`curl -X PUT "${window.location.origin}/api/v1/settings" \\
  -H "Authorization: Bearer YOUR_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{"dns": "1.1.1.1", "allowed_ips": "0.0.0.0/0"}'`}
                    </pre>
                  </div>
                </div>

                <div className="api-section">
                  <h4>Response Format</h4>
                  <p>All responses are in JSON format. Successful responses return the requested data. Error responses include an error message:</p>
                  <pre className="code-block">
{`{
  "error": "Error message here"
}`}
                  </pre>
                </div>

                <div className="api-section">
                  <h4>Base URL</h4>
                  <code className="base-url">{window.location.origin}</code>
                </div>
              </div>
            )}
          </div>
        </section>

        {/* Tailscale Section */}
        <section className="settings-section">
          <h2>
            <Network size={18} />
            Tailscale Integration
          </h2>

          {tailscaleMessage && (
            <div className={`message ${tailscaleMessage.type}`}>
              {tailscaleMessage.type === 'info' && tailscaleMessage.text.includes('http') ? (
                <>
                  <span>Please authenticate: </span>
                  <a
                    href={tailscaleMessage.text.replace('Please authenticate: ', '')}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="auth-link"
                  >
                    Open Auth URL <ExternalLink size={14} />
                  </a>
                </>
              ) : (
                tailscaleMessage.text
              )}
            </div>
          )}

          {!tailscale?.installed ? (
            <div className="form-group">
              <p className="hint">Tailscale is not installed on this system.</p>
            </div>
          ) : (
            <>
              <div className="form-group">
                <label>Status</label>
                <div className="tailscale-status">
                  <span className={`status-badge ${tailscale.connected ? 'online' : 'offline'}`}>
                    {tailscale.connected ? 'Connected' : tailscale.backend_state || 'Disconnected'}
                  </span>
                  {tailscale.self && (
                    <span className="tailscale-ip">{tailscale.self.tailscale_ip}</span>
                  )}
                </div>
              </div>

              <div className="form-group">
                <label>Connection</label>
                <div className="tailscale-actions">
                  {tailscale.connected ? (
                    <button
                      type="button"
                      onClick={handleTailscaleDisconnect}
                      className="btn-danger"
                      disabled={tailscaleLoading}
                    >
                      {tailscaleLoading ? <Loader2 size={16} className="spin" /> : <PowerOff size={16} />}
                      Disconnect
                    </button>
                ) : (
                  <button
                    type="button"
                    onClick={handleTailscaleConnect}
                    className="btn-primary"
                    disabled={tailscaleLoading}
                  >
                    {tailscaleLoading ? <Loader2 size={16} className="spin" /> : <Power size={16} />}
                    Connect
                  </button>
                )}
              </div>
            </div>

            {tailscale.connected && (
              <>
                <div className="form-group">
                  <label className="toggle-label">
                    <span>Route WireGuard to Tailscale</span>
                    <div className="toggle-switch">
                      <input
                        type="checkbox"
                        checked={tailscale.routing_enabled}
                        onChange={e => e.target.checked ? handleEnableRouting() : handleDisableRouting()}
                        disabled={tailscaleLoading}
                      />
                      <span className="toggle-slider"></span>
                    </div>
                  </label>
                  <span className="hint">
                    Allow WireGuard clients to access Tailscale network and advertised routes.
                  </span>
                </div>

                {tailscale.routes && tailscale.routes.length > 0 && (
                  <div className="form-group">
                    <label>Available Routes</label>
                    <div className="routes-list">
                      {tailscale.routes.map((route, idx) => (
                        <div key={idx} className="route-item">
                          <Router size={14} />
                          <code>{route.subnet}</code>
                          <span className="route-peer">via {route.peer_name}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {tailscale.peers && tailscale.peers.length > 0 && (
                  <div className="form-group">
                    <label>Tailscale Peers ({tailscale.peers.length})</label>
                    <div className="peers-list">
                      {tailscale.peers.map((peer, idx) => (
                        <div key={idx} className="peer-item">
                          <span className={`peer-status ${peer.online ? 'online' : 'offline'}`}></span>
                          <span className="peer-name">{peer.name}</span>
                          <code className="peer-ip">{peer.tailscale_ip}</code>
                          {peer.primary_routes && peer.primary_routes.length > 0 && (
                            <span className="peer-routes">
                              {peer.primary_routes.join(', ')}
                            </span>
                          )}
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </>
            )}
          </>
        )}
        </section>

        <div className="settings-actions">
          <button type="submit" className="save-btn" disabled={saving}>
            {saving ? (
              <>
                <Loader2 size={18} className="spin" />
                Saving...
              </>
            ) : (
              <>
                <Save size={18} />
                Save Changes
              </>
            )}
          </button>
        </div>
      </form>
    </div>
  )
}
