import { useState, useEffect, useRef, useCallback } from 'react'
import { api, TailscaleStatus, TailscaleUpdateStatus } from '../api/client'
import Layout from '../components/Layout'
import Modal from '../components/Modal'
import { Icon } from '../components/Icon'
import TailscaleOptions from '../components/TailscaleOptions'

type Notice = { kind: 'ok' | 'err' | 'info'; text: string; link?: string }

export default function TailscalePage({ onLogout }: { onLogout: () => void }) {
  const [ts, setTs] = useState<TailscaleStatus | null>(null)
  const [upd, setUpd] = useState<TailscaleUpdateStatus | null>(null)
  const [loaded, setLoaded] = useState(false)
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState<Notice | null>(null)
  const [updError, setUpdError] = useState<string | null>(null)
  const [confirmUpdate, setConfirmUpdate] = useState(false)
  const pollRef = useRef<number | undefined>(undefined)

  const fetchStatus = useCallback(async () => {
    try {
      setTs(await api.getTailscaleStatus())
    } catch (err) {
      console.error('Failed to load Tailscale status:', err)
    }
  }, [])

  const fetchUpdate = useCallback(async () => {
    try {
      setUpd(await api.getTailscaleUpdateStatus())
    } catch (err) {
      console.error('Failed to load update status:', err)
    }
  }, [])

  useEffect(() => {
    Promise.all([fetchStatus(), fetchUpdate()]).finally(() => setLoaded(true))
    return () => window.clearInterval(pollRef.current)
  }, [fetchStatus, fetchUpdate])

  // Poll while a version check or update runs (an update restarts tailscaled)
  const updBusy = !!(upd && (upd.checking || upd.updating))
  useEffect(() => {
    if (!updBusy) return
    const timer = window.setInterval(async () => {
      try {
        const st = await api.getTailscaleUpdateStatus()
        setUpd(st)
        if (!st.checking && !st.updating) fetchStatus()
      } catch {
        /* panel may be briefly unreachable while tailscaled restarts */
      }
    }, 3000)
    return () => window.clearInterval(timer)
  }, [updBusy, fetchStatus])

  const run = async (fn: () => Promise<void>, fallback: string) => {
    setBusy(true)
    setNotice(null)
    try {
      await fn()
    } catch (err) {
      setNotice({ kind: 'err', text: err instanceof Error ? err.message : fallback })
    } finally {
      setBusy(false)
    }
  }

  const connect = () =>
    run(async () => {
      const result = await api.connectTailscale()
      if (result.auth_url) {
        setNotice({ kind: 'info', text: 'Authentication required. Sign in to Tailscale to finish connecting.', link: result.auth_url })
        window.open(result.auth_url, '_blank', 'noopener,noreferrer')
        window.clearInterval(pollRef.current)
        pollRef.current = window.setInterval(async () => {
          try {
            const status = await api.getTailscaleStatus()
            setTs(status)
            if (status.connected) {
              window.clearInterval(pollRef.current)
              setNotice({ kind: 'ok', text: 'Connected to Tailscale.' })
            }
          } catch {
            /* keep polling */
          }
        }, 3000)
        window.setTimeout(() => window.clearInterval(pollRef.current), 120000)
      } else {
        setNotice({ kind: 'ok', text: result.message })
      }
      await fetchStatus()
    }, 'Failed to connect')

  const disconnect = () =>
    run(async () => {
      const r = await api.disconnectTailscale()
      setNotice({ kind: 'ok', text: r.message })
      await fetchStatus()
    }, 'Failed to disconnect')

  const setRouting = (enable: boolean) =>
    run(async () => {
      const r = enable ? await api.enableTailscaleRouting() : await api.disableTailscaleRouting()
      setNotice({ kind: 'ok', text: r.message })
      await fetchStatus()
    }, 'Failed to change routing')

  const checkUpdate = async () => {
    setUpdError(null)
    try {
      await api.checkTailscaleUpdate()
      setUpd(p => (p ? { ...p, checking: true } : p))
    } catch (err) {
      setUpdError(err instanceof Error ? err.message : 'Failed to check for updates')
    }
  }

  const applyUpdate = async () => {
    setConfirmUpdate(false)
    setUpdError(null)
    try {
      await api.applyTailscaleUpdate()
      setUpd(p => (p ? { ...p, updating: true } : p))
    } catch (err) {
      setUpdError(err instanceof Error ? err.message : 'Failed to start update')
    }
  }

  const toggleAuto = async (enabled: boolean) => {
    setUpdError(null)
    try {
      await api.setTailscaleAutoUpdate(enabled)
      setUpd(p => (p ? { ...p, auto_update: enabled } : p))
    } catch (err) {
      setUpdError(err instanceof Error ? err.message : 'Failed to change auto-update')
    }
  }

  const stateBadge = ts?.connected ? (
    <span className="badge success">connected</span>
  ) : (
    <span className="badge warning">{ts?.backend_state || 'disconnected'}</span>
  )

  return (
    <Layout onLogout={onLogout}>
      {notice && (
        <div className={`msg msg-${notice.kind}`} role={notice.kind === 'err' ? 'alert' : 'status'}>
          <Icon name={notice.kind === 'ok' ? 'check-circle' : notice.kind === 'err' ? 'x-circle' : 'info'} />
          <div className="msg-body">
            {notice.text}
            {notice.link && (
              <>
                {' '}
                <a href={notice.link} target="_blank" rel="noopener noreferrer">Open sign-in page <Icon name="external" /></a>
              </>
            )}
          </div>
          <button type="button" className="btn btn-ghost btn-icon btn-sm msg-close" aria-label="Dismiss" onClick={() => setNotice(null)}>
            <Icon name="x" />
          </button>
        </div>
      )}

      {!loaded ? (
        <div className="empty-state"><Icon name="refresh" />Loading…</div>
      ) : !ts?.installed ? (
        <section className="panel">
          <div className="empty-state"><Icon name="network" />Tailscale is not installed on this system.</div>
        </section>
      ) : (
        <>
          <div className="cols cols-2">
            <section className="panel">
              <header className="panel-h">
                <h2><Icon name="network" />Connection</h2>
                <div className="panel-tools">{stateBadge}</div>
              </header>
              <div className="panel-b">
                <dl className="props">
                  <dt>Status</dt><dd>{stateBadge}</dd>
                  <dt>Tailscale IP</dt><dd className="mono">{ts.self?.tailscale_ip || '—'}</dd>
                  <dt>Hostname</dt><dd>{ts.self?.hostname || '—'}</dd>
                  <dt>Peers</dt><dd>{ts.peers?.length ?? 0}</dd>
                </dl>
                <div className="form-actions">
                  {ts.connected ? (
                    <button type="button" className={busy ? 'btn btn-danger is-busy' : 'btn btn-danger'} disabled={busy} onClick={disconnect}>
                      <Icon name="stop" />
                      Disconnect
                    </button>
                  ) : (
                    <button type="button" className={busy ? 'btn btn-primary is-busy' : 'btn btn-primary'} disabled={busy} onClick={connect}>
                      <Icon name="play" />
                      Connect
                    </button>
                  )}
                </div>
              </div>
            </section>

            <section className="panel">
              <header className="panel-h"><h2><Icon name="zap" />Routing</h2></header>
              <div className="panel-b">
                <label className="check">
                  <input
                    type="checkbox"
                    checked={!!ts.routing_enabled}
                    disabled={busy || !ts.connected}
                    onChange={e => setRouting(e.target.checked)}
                  />
                  <span>
                    Route WireGuard clients into Tailscale
                    <span className="hint">
                      {ts.connected
                        ? 'Clients can reach tailnet peers (100.x.y.z) and advertised subnet routes without installing Tailscale. Rules are restored automatically after a restart.'
                        : 'Connect Tailscale first to enable routing.'}
                    </span>
                  </span>
                </label>
              </div>
            </section>
          </div>

          {upd && (
            <section className="panel">
              <header className="panel-h">
                <h2><Icon name="download" />Version & updates</h2>
                <div className="panel-tools">
                  {upd.updating ? (
                    <span className="badge running">updating</span>
                  ) : upd.checking ? (
                    <span className="badge running">checking</span>
                  ) : upd.update_available ? (
                    <span className="badge info">update available</span>
                  ) : upd.latest_version ? (
                    <span className="badge success">up to date</span>
                  ) : null}
                </div>
              </header>
              <div className="panel-b">
                {updError && (
                  <div className="msg msg-err compact" role="alert"><Icon name="x-circle" /><div className="msg-body">{updError}</div></div>
                )}
                <dl className="props">
                  <dt>Installed</dt><dd className="mono">{upd.current_version || 'unknown'}</dd>
                  <dt>Latest stable</dt><dd className="mono">{upd.latest_version || '—'}</dd>
                  <dt>Last check</dt><dd>{upd.checked_at ? new Date(upd.checked_at).toLocaleString() : 'not checked yet'}</dd>
                  {upd.last_result && (
                    <>
                      <dt>Last update</dt>
                      <dd>
                        <span className={upd.last_result.success ? 'badge success' : upd.last_result.rolled_back ? 'badge warning' : 'badge failed'}>
                          {upd.last_result.success ? 'success' : upd.last_result.rolled_back ? 'rolled back' : 'failed'}
                        </span>
                        <span className="dim">
                          {new Date(upd.last_result.time).toLocaleString()} · {upd.last_result.auto ? 'automatic' : 'manual'}
                        </span>
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
                      <input type="checkbox" checked={upd.auto_update} disabled={updBusy} onChange={e => toggleAuto(e.target.checked)} />
                      <span>
                        Automatic updates (stable channel)
                        <span className="hint">
                          Checks every 6 hours, verifies the new version after restarting tailscaled and rolls back automatically if anything fails.
                        </span>
                      </span>
                    </label>
                    <div className="form-actions">
                      <button type="button" className="btn" disabled={updBusy} onClick={checkUpdate}>
                        <Icon name="refresh" />
                        Check for updates
                      </button>
                      {upd.update_available && (
                        <button type="button" className="btn btn-primary" disabled={updBusy} onClick={() => setConfirmUpdate(true)}>
                          <Icon name="download" />
                          Update to {upd.latest_version}
                        </button>
                      )}
                    </div>
                  </>
                ) : (
                  <div className="msg msg-info compact mt">
                    <Icon name="info" />
                    <div className="msg-body">Automatic updates are unavailable: {upd.unsupported_reason}</div>
                  </div>
                )}
              </div>
            </section>
          )}

          {ts.connected && <TailscaleOptions />}

          {ts.connected && ts.routes && ts.routes.length > 0 && (
            <section className="panel">
              <header className="panel-h"><h2><Icon name="link" />Available routes</h2><div className="panel-tools"><span className="badge">{ts.routes.length}</span></div></header>
              <div className="tbl-wrap">
                <table className="tbl tbl-compact">
                  <thead><tr><th>Subnet</th><th>Advertised by</th></tr></thead>
                  <tbody>{ts.routes.map((r, i) => <tr key={i}><td className="mono">{r.subnet}</td><td>{r.peer_name}</td></tr>)}</tbody>
                </table>
              </div>
            </section>
          )}

          {ts.connected && ts.peers && ts.peers.length > 0 && (
            <section className="panel">
              <header className="panel-h"><h2><Icon name="users" />Tailnet peers</h2><div className="panel-tools"><span className="badge">{ts.peers.length}</span></div></header>
              <div className="tbl-wrap">
                <table className="tbl tbl-compact">
                  <thead><tr><th>Peer</th><th>Address</th><th>Routes</th></tr></thead>
                  <tbody>
                    {ts.peers.map((p, i) => (
                      <tr key={i}>
                        <td><div className="name-cell"><i className={p.online ? 'dot ok' : 'dot'} /><strong>{p.name}</strong></div></td>
                        <td className="mono">{p.tailscale_ip}</td>
                        <td className="mono">{p.primary_routes?.join(', ') || '—'}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </section>
          )}
        </>
      )}

      {confirmUpdate && upd && (
        <Modal
          title={`Update Tailscale to ${upd.latest_version}?`}
          icon="download"
          onClose={() => setConfirmUpdate(false)}
          footer={
            <>
              <button type="button" className="btn" onClick={() => setConfirmUpdate(false)}>Cancel</button>
              <button type="button" className="btn btn-primary" onClick={applyUpdate}>Update now</button>
            </>
          }
        >
          tailscaled restarts and the Tailscale link drops for a few seconds. WireGuard clients stay connected. If the new version fails verification, the previous version ({upd.current_version}) is restored automatically.
        </Modal>
      )}
    </Layout>
  )
}
