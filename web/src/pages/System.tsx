import { useState, useEffect, useCallback } from 'react'
import { api } from '../api/client'
import type { SystemStatus, WireGuardUpdateStatus } from '../api/client'
import Layout from '../components/Layout'
import Modal from '../components/Modal'
import { Icon } from '../components/Icon'

function formatUptime(sec: number): string {
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  return d > 0 ? `${d}d ${h}h` : h > 0 ? `${h}h ${m}m` : `${m}m`
}

export default function SystemPage({ onLogout }: { onLogout: () => void }) {
  const [sys, setSys] = useState<SystemStatus | null>(null)
  const [upd, setUpd] = useState<WireGuardUpdateStatus | null>(null)
  const [loaded, setLoaded] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [confirm, setConfirm] = useState(false)

  const fetchAll = useCallback(async () => {
    try {
      const [s, u] = await Promise.all([api.getSystemStatus(), api.getWireGuardUpdateStatus()])
      setSys(s)
      setUpd(u)
    } catch (err) {
      console.error('Failed to load system status:', err)
      setError('Could not load system status.')
    } finally {
      setLoaded(true)
    }
  }, [])

  useEffect(() => {
    fetchAll()
    const t = window.setInterval(fetchAll, 15000)
    return () => window.clearInterval(t)
  }, [fetchAll])

  // Poll faster while a check or update runs
  const busy = !!(upd && (upd.checking || upd.updating))
  useEffect(() => {
    if (!busy) return
    const t = window.setInterval(async () => {
      try {
        setUpd(await api.getWireGuardUpdateStatus())
      } catch {
        /* keep polling */
      }
    }, 3000)
    return () => window.clearInterval(t)
  }, [busy])

  const act = async (fn: () => Promise<unknown>, fallback: string) => {
    setError(null)
    try {
      await fn()
      setUpd(await api.getWireGuardUpdateStatus())
    } catch (err) {
      setError(err instanceof Error ? err.message : fallback)
    }
  }

  const toggleAuto = (enabled: boolean) =>
    act(async () => {
      await api.setWireGuardAutoUpdate(enabled)
      setUpd(p => (p ? { ...p, auto_update: enabled } : p))
    }, 'Failed to change auto-update')

  return (
    <Layout onLogout={onLogout}>
      {error && (
        <div className="msg msg-err" role="alert">
          <Icon name="x-circle" />
          <div className="msg-body">{error}</div>
          <button type="button" className="btn btn-ghost btn-icon btn-sm msg-close" aria-label="Dismiss" onClick={() => setError(null)}><Icon name="x" /></button>
        </div>
      )}

      {!loaded ? (
        <div className="empty-state"><Icon name="refresh" />Loading…</div>
      ) : (
        <>
          {sys && (
            <div className="vitals">
              <section className="vital" aria-label="WireGuard interface">
                <div className="vital-h"><span className="vital-t"><Icon name="shield" />Interface {sys.interface.name}</span></div>
                <div className="vital-v"><span className={sys.interface.up ? 'badge success' : 'badge failed'}>{sys.interface.up ? 'up' : 'down'}</span></div>
                <div className="vital-s">{sys.interface.listen_port ? `UDP ${sys.interface.listen_port} in the container` : 'not running'}</div>
              </section>
              <section className="vital" aria-label="Clients">
                <div className="vital-h"><span className="vital-t"><Icon name="users" />Clients</span></div>
                <div className="vital-v">{sys.clients.online}<span className="dim"> / {sys.clients.enabled}</span></div>
                <div className="vital-s">online / enabled · {sys.clients.total} total{sys.clients.expired > 0 ? ` · ${sys.clients.expired} expired` : ''}</div>
              </section>
              <section className="vital" aria-label="Uptime">
                <div className="vital-h"><span className="vital-t"><Icon name="clock" />Panel uptime</span></div>
                <div className="vital-v">{formatUptime(sys.uptime_seconds)}</div>
                <div className="vital-s">since the last start</div>
              </section>
            </div>
          )}

          <div className="cols cols-2">
            {sys && (
              <section className="panel">
                <header className="panel-h"><h2><Icon name="server" />Versions</h2></header>
                <div className="panel-b">
                  <dl className="props">
                    <dt>Panel</dt><dd className="mono">{sys.panel_version}</dd>
                    <dt>WireGuard tools</dt><dd className="mono">{sys.wireguard_tools || 'unknown'}</dd>
                    <dt>Tailscale</dt><dd className="mono">{sys.tailscale_version || 'not installed'}</dd>
                    <dt>Host kernel</dt><dd className="mono">{sys.kernel || 'unknown'}</dd>
                    <dt>Runtime</dt><dd className="mono">{sys.go_version}</dd>
                  </dl>
                  <p className="hint">The WireGuard data path runs in the host kernel and is updated with the host. Only the userspace tools are updated here.</p>
                </div>
              </section>
            )}

            {upd && (
              <section className="panel">
                <header className="panel-h">
                  <h2><Icon name="download" />WireGuard tools updates</h2>
                  <div className="panel-tools">
                    {upd.updating ? <span className="badge running">updating</span>
                      : upd.checking ? <span className="badge running">checking</span>
                      : upd.update_available ? <span className="badge info">update available</span>
                      : upd.latest_version ? <span className="badge success">up to date</span> : null}
                  </div>
                </header>
                <div className="panel-b">
                  <dl className="props">
                    <dt>Installed</dt><dd className="mono">{upd.current_version || 'unknown'}</dd>
                    <dt>Latest package</dt><dd className="mono">{upd.latest_version || '—'}</dd>
                    <dt>Last check</dt><dd>{upd.checked_at ? new Date(upd.checked_at).toLocaleString() : 'not checked yet'}</dd>
                    {upd.last_result && (
                      <>
                        <dt>Last update</dt>
                        <dd>
                          <span className={upd.last_result.success ? 'badge success' : upd.last_result.rolled_back ? 'badge warning' : 'badge failed'}>
                            {upd.last_result.success ? 'success' : upd.last_result.rolled_back ? 'rolled back' : 'failed'}
                          </span>
                          <span className="dim">{new Date(upd.last_result.time).toLocaleString()} · {upd.last_result.auto ? 'automatic' : 'manual'}</span>
                          <span>{upd.last_result.message}</span>
                        </dd>
                      </>
                    )}
                  </dl>
                  {upd.check_error && !upd.checking && (
                    <div className="msg msg-warn compact"><Icon name="alert" /><div className="msg-body">Last check failed: {upd.check_error}</div></div>
                  )}
                  {upd.supported ? (
                    <>
                      <label className="check mt">
                        <input type="checkbox" checked={upd.auto_update} disabled={busy} onChange={e => toggleAuto(e.target.checked)} />
                        <span>
                          Automatic updates (stable channel)
                          <span className="hint">Checks every 6 hours. Tunnels and clients stay connected; the update is verified and rolled back automatically if anything fails.</span>
                        </span>
                      </label>
                      <div className="form-actions">
                        <button type="button" className="btn" disabled={busy} onClick={() => act(() => api.checkWireGuardUpdate(), 'Failed to check for updates')}>
                          <Icon name="refresh" />Check for updates
                        </button>
                        {upd.update_available && (
                          <button type="button" className="btn btn-primary" disabled={busy} onClick={() => setConfirm(true)}>
                            <Icon name="download" />Update now
                          </button>
                        )}
                      </div>
                    </>
                  ) : (
                    <div className="msg msg-info compact mt"><Icon name="info" /><div className="msg-body">Updates are unavailable: {upd.unsupported_reason}</div></div>
                  )}
                </div>
              </section>
            )}
          </div>
        </>
      )}

      {confirm && (
        <Modal
          title="Update WireGuard tools"
          icon="download"
          onClose={() => setConfirm(false)}
          footer={
            <>
              <button type="button" className="btn" onClick={() => setConfirm(false)}>Cancel</button>
              <button type="button" className="btn btn-primary" onClick={() => { setConfirm(false); act(() => api.applyWireGuardUpdate(), 'Failed to start update') }}>Update</button>
            </>
          }
        >
          <p>The userspace tools are replaced in place. The WireGuard interface and all client tunnels stay up, and the previous version is restored automatically if the check afterwards fails.</p>
        </Modal>
      )}
    </Layout>
  )
}
