import { useState, useEffect, useCallback } from 'react'
import { api, Peer, ConnectionLog } from '../api/client'
import Layout from '../components/Layout'
import Modal from '../components/Modal'
import { Icon } from '../components/Icon'
import { formatBytes, timeAgo } from '../components/format'

const LOGS_PER_PAGE = 10

// Earliest selectable expiry (tomorrow, UTC), YYYY-MM-DD
const tomorrow = () => new Date(Date.now() + 86400000).toISOString().slice(0, 10)

export default function Clients({ onLogout }: { onLogout: () => void }) {
  const [peers, setPeers] = useState<Peer[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [loggingEnabled, setLoggingEnabled] = useState(false)

  const [showAdd, setShowAdd] = useState(false)
  const [newName, setNewName] = useState('')
  const [newNote, setNewNote] = useState('')
  const [newExpiry, setNewExpiry] = useState('')
  const [adding, setAdding] = useState(false)

  const [editPeer, setEditPeer] = useState<Peer | null>(null)
  const [editName, setEditName] = useState('')
  const [editNote, setEditNote] = useState('')
  const [editExpiry, setEditExpiry] = useState('')
  const [saving, setSaving] = useState(false)

  const [qr, setQr] = useState<{ ip: string; name: string; url: string } | null>(null)
  const [logsFor, setLogsFor] = useState<Peer | null>(null)
  const [logs, setLogs] = useState<ConnectionLog[]>([])
  const [logsLoading, setLogsLoading] = useState(false)
  const [logsPage, setLogsPage] = useState(0)
  const [toDelete, setToDelete] = useState<Peer | null>(null)

  const fetchPeers = useCallback(async () => {
    try {
      setPeers(await api.getPeers())
      setError(null)
    } catch (err) {
      console.error('Failed to load clients:', err)
      setError('Could not load clients. Retrying…')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchPeers()
    api.getSettings().then(s => setLoggingEnabled(s.logging_enabled)).catch(() => {})
    const interval = setInterval(() => { if (!document.hidden) fetchPeers() }, 5000)
    return () => clearInterval(interval)
  }, [fetchPeers])

  const handleAdd = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newName.trim()) return
    setAdding(true)
    try {
      await api.createPeer(newName.trim(), { note: newNote.trim(), expires_at: newExpiry || undefined })
      setNewName('')
      setNewNote('')
      setNewExpiry('')
      setShowAdd(false)
      await fetchPeers()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create client')
    } finally {
      setAdding(false)
    }
  }

  const handleToggle = async (peer: Peer) => {
    try {
      await api.updatePeer(peer.assigned_ip, { enabled: !peer.enabled })
      await fetchPeers()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to change client state')
    }
  }

  const openEdit = (peer: Peer) => {
    setEditPeer(peer)
    setEditName(peer.name)
    setEditNote(peer.note || '')
    setEditExpiry(peer.expires_at ? peer.expires_at.slice(0, 10) : '')
  }

  const handleEditSave = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!editPeer || !editName.trim()) return
    setSaving(true)
    try {
      const data: { name?: string; note?: string; expires_at?: string } = {}
      if (editName.trim() !== editPeer.name) data.name = editName.trim()
      if (editNote.trim() !== (editPeer.note || '')) data.note = editNote.trim()
      const oldExpiry = editPeer.expires_at ? editPeer.expires_at.slice(0, 10) : ''
      if (editExpiry !== oldExpiry) data.expires_at = editExpiry
      if (Object.keys(data).length > 0) await api.updatePeer(editPeer.assigned_ip, data)
      setEditPeer(null)
      await fetchPeers()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update client')
      setEditPeer(null)
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    if (!toDelete) return
    try {
      await api.deletePeer(toDelete.assigned_ip)
      await fetchPeers()
    } catch {
      setError('Failed to delete client')
    }
    setToDelete(null)
  }

  const handleDownload = async (peer: Peer) => {
    try {
      await api.downloadConfig(peer.assigned_ip, `${peer.name}.conf`)
    } catch {
      setError('Failed to download config')
    }
  }

  const showQr = async (peer: Peer) => {
    try {
      const url = await api.getQRCode(peer.assigned_ip)
      setQr({ ip: peer.assigned_ip, name: peer.name, url })
    } catch {
      setError('Failed to generate QR code')
    }
  }

  const closeQr = () => setQr(null)

  const showLogs = async (peer: Peer) => {
    setLogsFor(peer)
    setLogsPage(0)
    setLogsLoading(true)
    try {
      setLogs(await api.getPeerLogs(peer.assigned_ip))
    } catch {
      setLogs([])
    } finally {
      setLogsLoading(false)
    }
  }

  const onlineCount = peers.filter(p => p.is_online).length
  const totalTransfer = peers.reduce((acc, p) => acc + p.transfer_rx + p.transfer_tx, 0)
  const totalPages = Math.max(1, Math.ceil(logs.length / LOGS_PER_PAGE))
  const pagedLogs = logs.slice(logsPage * LOGS_PER_PAGE, (logsPage + 1) * LOGS_PER_PAGE)

  const actions = (
    <>
      <button type="button" className="btn" onClick={fetchPeers}>
        <Icon name="refresh" />
        Refresh
      </button>
      <button type="button" className="btn btn-primary" onClick={() => setShowAdd(true)}>
        <Icon name="plus" />
        New client
      </button>
    </>
  )

  return (
    <Layout onLogout={onLogout} actions={actions}>
      {error && (
        <div className="msg msg-err" role="alert">
          <Icon name="x-circle" />
          <div className="msg-body">{error}</div>
          <button type="button" className="btn btn-ghost btn-icon btn-sm msg-close" aria-label="Dismiss" onClick={() => setError(null)}>
            <Icon name="x" />
          </button>
        </div>
      )}

      <div className="vitals vitals-3">
        <section className="vital" aria-label="Clients">
          <div className="vital-h"><span className="vital-t"><Icon name="users" />Clients</span></div>
          <div className="vital-v">{peers.length}</div>
          <div className="vital-s">{peers.filter(p => !p.enabled).length} disabled</div>
        </section>
        <section className="vital" aria-label="Connected">
          <div className="vital-h"><span className="vital-t"><Icon name="wifi" />Connected</span></div>
          <div className="vital-v">{onlineCount}</div>
          <div className="vital-s">active in the last 3 minutes</div>
        </section>
        <section className="vital" aria-label="Transfer">
          <div className="vital-h"><span className="vital-t"><Icon name="activity" />Total transfer</span></div>
          <div className="vital-v">{formatBytes(totalTransfer)}</div>
          <div className="vital-s">
            ↓ {formatBytes(peers.reduce((a, p) => a + p.transfer_rx, 0))} · ↑ {formatBytes(peers.reduce((a, p) => a + p.transfer_tx, 0))}
          </div>
        </section>
      </div>

      <section className="panel">
        <header className="panel-h">
          <h2><Icon name="users" />All clients</h2>
          <div className="panel-tools"><span className="badge">{peers.length}</span></div>
        </header>
        {loading && peers.length === 0 ? (
          <div className="empty-state"><Icon name="refresh" />Loading clients…</div>
        ) : peers.length === 0 ? (
          <div className="empty-state">
            <Icon name="users" />
            No clients yet. Create the first one to get a QR code and a config file.
            <div className="mt"><button type="button" className="btn btn-primary" onClick={() => setShowAdd(true)}><Icon name="plus" />New client</button></div>
          </div>
        ) : (
          <div className="tbl-wrap">
            <table className="tbl">
              <thead>
                <tr>
                  <th>Client</th>
                  <th>Address</th>
                  <th>Last handshake</th>
                  <th className="num">Received</th>
                  <th className="num">Sent</th>
                  <th>Status</th>
                  <th><span className="sr-only">Actions</span></th>
                </tr>
              </thead>
              <tbody>
                {peers.map(peer => (
                  <tr key={peer.id} className={peer.enabled ? undefined : 'row-off'}>
                    <td>
                      <div className="name-cell">
                        <i className={peer.is_online ? 'dot ok' : 'dot'} title={peer.is_online ? 'Online' : 'Offline'} />
                        <strong>{peer.name}</strong>
                        <button
                          type="button"
                          className="btn btn-ghost btn-icon btn-sm"
                          aria-label={`Edit ${peer.name}`}
                          title="Edit"
                          onClick={() => openEdit(peer)}
                        >
                          <Icon name="edit" />
                        </button>
                      </div>
                      {peer.note && <div className="note-line" title={peer.note}>{peer.note}</div>}
                    </td>
                    <td className="mono">{peer.assigned_ip}</td>
                    <td>
                      <span title={peer.endpoint || undefined}>{timeAgo(peer.latest_handshake)}</span>
                    </td>
                    <td className="num"><Icon name="arrow-down" className="ic-inline" /> {formatBytes(peer.transfer_rx)}</td>
                    <td className="num"><Icon name="arrow-up" className="ic-inline" /> {formatBytes(peer.transfer_tx)}</td>
                    <td>
                      {peer.expired ? (
                        <span className="badge failed" title={peer.expires_at}>expired</span>
                      ) : !peer.enabled ? (
                        <span className="badge inactive">disabled</span>
                      ) : peer.is_online ? (
                        <span className="badge success">online</span>
                      ) : (
                        <span className="badge">offline</span>
                      )}
                      {peer.expires_at && !peer.expired && (
                        <div className="note-line" title={peer.expires_at}>expires {peer.expires_at.slice(0, 10)}</div>
                      )}
                    </td>
                    <td className="actions-cell">
                      <div className="btn-group">
                        <button type="button" className="btn btn-ghost btn-icon btn-sm" title={peer.enabled ? 'Disable' : 'Enable'} aria-label={peer.enabled ? `Disable ${peer.name}` : `Enable ${peer.name}`} onClick={() => handleToggle(peer)}>
                          <Icon name="power" />
                        </button>
                        <button type="button" className="btn btn-ghost btn-icon btn-sm" title="QR code" aria-label={`QR code for ${peer.name}`} onClick={() => showQr(peer)}>
                          <Icon name="qr" />
                        </button>
                        {loggingEnabled && (
                          <button type="button" className="btn btn-ghost btn-icon btn-sm" title="Connection log" aria-label={`Connection log for ${peer.name}`} onClick={() => showLogs(peer)}>
                            <Icon name="file-text" />
                          </button>
                        )}
                        <button type="button" className="btn btn-ghost btn-icon btn-sm" title="Download config" aria-label={`Download config for ${peer.name}`} onClick={() => handleDownload(peer)}>
                          <Icon name="download" />
                        </button>
                        <button type="button" className="btn btn-ghost btn-icon btn-sm btn-danger" title="Delete" aria-label={`Delete ${peer.name}`} onClick={() => setToDelete(peer)}>
                          <Icon name="trash" />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {editPeer && (
        <Modal
          title="Edit client"
          icon="edit"
          onClose={() => setEditPeer(null)}
          footer={
            <>
              <button type="button" className="btn" onClick={() => setEditPeer(null)}>Cancel</button>
              <button type="submit" form="edit-client" className={saving ? 'btn btn-primary is-busy' : 'btn btn-primary'} disabled={saving || !editName.trim()}>Save changes</button>
            </>
          }
        >
          <form id="edit-client" onSubmit={handleEditSave}>
            <label className="field">
              <span className="lbl">Name</span>
              <input type="text" value={editName} onChange={e => setEditName(e.target.value)} maxLength={64} required autoFocus />
            </label>
            <label className="field">
              <span className="lbl">Note <span className="opt">optional</span></span>
              <input type="text" value={editNote} onChange={e => setEditNote(e.target.value)} maxLength={200} />
            </label>
            <label className="field">
              <span className="lbl">Expires <span className="opt">optional</span></span>
              <input type="date" value={editExpiry} min={tomorrow()} onChange={e => setEditExpiry(e.target.value)} />
              <span className="hint">Clear the date to keep the client active indefinitely. An expired client is re-enabled after setting a new date.</span>
            </label>
          </form>
        </Modal>
      )}

      {showAdd && (
        <Modal
          title="New client"
          icon="plus"
          onClose={() => setShowAdd(false)}
          footer={
            <>
              <button type="button" className="btn" onClick={() => setShowAdd(false)}>Cancel</button>
              <button type="submit" form="add-client" className={adding ? 'btn btn-primary is-busy' : 'btn btn-primary'} disabled={adding || !newName.trim()}>Create client</button>
            </>
          }
        >
          <form id="add-client" onSubmit={handleAdd}>
            <label className="field">
              <span className="lbl">Name</span>
              <input type="text" value={newName} onChange={e => setNewName(e.target.value)} placeholder="e.g. Phone" maxLength={64} required autoFocus />
              <span className="hint">The next free address in the VPN subnet is assigned automatically.</span>
            </label>
            <label className="field">
              <span className="lbl">Note <span className="opt">optional</span></span>
              <input type="text" value={newNote} onChange={e => setNewNote(e.target.value)} placeholder="e.g. Alice's laptop" maxLength={200} />
            </label>
            <label className="field">
              <span className="lbl">Expires <span className="opt">optional</span></span>
              <input type="date" value={newExpiry} min={tomorrow()} onChange={e => setNewExpiry(e.target.value)} />
              <span className="hint">The client is disabled automatically at the end of that day (UTC).</span>
            </label>
          </form>
        </Modal>
      )}

      {qr && (
        <Modal
          title={`QR code · ${qr.name}`}
          icon="qr"
          onClose={closeQr}
          footer={<button type="button" className="btn" onClick={closeQr}>Close</button>}
        >
          <div className="qr-wrap">
            <img src={qr.url} alt={`WireGuard QR code for ${qr.name}`} className="qr-img" />
            <p className="hint">Scan with the WireGuard mobile app. Anyone who can see this code can use the client.</p>
          </div>
        </Modal>
      )}

      {logsFor && (
        <Modal
          title={`Connection log · ${logsFor.name}`}
          icon="file-text"
          wide
          onClose={() => setLogsFor(null)}
          footer={
            <>
              {logs.length > LOGS_PER_PAGE && (
                <div className="btn-group pager">
                  <button type="button" className="btn btn-sm" disabled={logsPage === 0} onClick={() => setLogsPage(p => p - 1)}><Icon name="chevron-left" />Prev</button>
                  <span className="dim">{logsPage + 1} / {totalPages}</span>
                  <button type="button" className="btn btn-sm" disabled={logsPage >= totalPages - 1} onClick={() => setLogsPage(p => p + 1)}>Next<Icon name="chevron-right" /></button>
                </div>
              )}
              <button type="button" className="btn" onClick={() => setLogsFor(null)}>Close</button>
            </>
          }
        >
          {logsLoading ? (
            <div className="empty-state"><Icon name="refresh" />Loading…</div>
          ) : logs.length === 0 ? (
            <div className="empty-state"><Icon name="file-text" />No connections recorded yet.</div>
          ) : (
            <div className="tbl-wrap">
              <table className="tbl tbl-compact">
                <thead><tr><th>Endpoint</th><th>Connected</th></tr></thead>
                <tbody>
                  {pagedLogs.map(l => (
                    <tr key={l.id}>
                      <td className="mono">{l.endpoint}</td>
                      <td>{new Date(l.connected_at).toLocaleString()}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Modal>
      )}

      {toDelete && (
        <Modal
          title="Delete client"
          icon="alert"
          danger
          onClose={() => setToDelete(null)}
          footer={
            <>
              <button type="button" className="btn" onClick={() => setToDelete(null)}>Cancel</button>
              <button type="button" className="btn btn-danger-solid" onClick={handleDelete}>Delete client</button>
            </>
          }
        >
          Delete “{toDelete.name}” ({toDelete.assigned_ip})? The client loses access immediately and its keys cannot be recovered.
        </Modal>
      )}
    </Layout>
  )
}
