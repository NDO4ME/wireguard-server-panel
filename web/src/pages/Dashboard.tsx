import { useState, useEffect, useCallback } from 'react'
import {
  Shield,
  Plus,
  Trash2,
  Download,
  QrCode,
  LogOut,
  RefreshCw,
  X,
  Check,
  Edit3,
  Power,
  ArrowDown,
  ArrowUp,
  Clock,
  Globe,
  Settings,
  FileText,
  ChevronLeft,
  ChevronRight
} from 'lucide-react'
import { api, Peer, ConnectionLog } from '../api/client'
import '../styles/dashboard.css'

interface DashboardProps {
  onLogout: () => void
  onSettings: () => void
}

// Format bytes to human readable
function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

// Format time ago
function timeAgo(dateString: string): string {
  if (!dateString || dateString === '0001-01-01T00:00:00Z') return 'Never'
  const date = new Date(dateString)
  const now = new Date()
  const seconds = Math.floor((now.getTime() - date.getTime()) / 1000)

  if (seconds < 60) return 'Just now'
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`
  return `${Math.floor(seconds / 86400)}d ago`
}

export default function Dashboard({ onLogout, onSettings }: DashboardProps) {
  const [peers, setPeers] = useState<Peer[]>([])
  const [loading, setLoading] = useState(true)
  const [showAddModal, setShowAddModal] = useState(false)
  const [showQRModal, setShowQRModal] = useState<string | null>(null)
  const [showLogsModal, setShowLogsModal] = useState<string | null>(null)
  const [peerLogs, setPeerLogs] = useState<ConnectionLog[]>([])
  const [logsLoading, setLogsLoading] = useState(false)
  const [loggingEnabled, setLoggingEnabled] = useState(false)
  const [logsPage, setLogsPage] = useState(0)
  const LOGS_PER_PAGE = 10
  const [editingPeer, setEditingPeer] = useState<string | null>(null)
  const [editName, setEditName] = useState('')
  const [newPeerName, setNewPeerName] = useState('')
  const [addingPeer, setAddingPeer] = useState(false)
  const [qrCodeUrl, setQrCodeUrl] = useState<string | null>(null)
  const [confirmModal, setConfirmModal] = useState<{
    show: boolean
    title: string
    message: string
    onConfirm: () => void
  } | null>(null)

  const fetchPeers = useCallback(async () => {
    try {
      const data = await api.getPeers()
      setPeers(data)
    } catch (err) {
      console.error('Failed to load peers:', err)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchPeers()
    // Check if logging is enabled
    api.getSettings().then(settings => {
      setLoggingEnabled(settings.logging_enabled)
    }).catch(() => {})
    // Auto-refresh every 5 seconds
    const interval = setInterval(fetchPeers, 5000)
    return () => clearInterval(interval)
  }, [fetchPeers])

  const handleShowLogs = async (ip: string) => {
    setLogsLoading(true)
    setShowLogsModal(ip)
    setLogsPage(0)
    try {
      const logs = await api.getPeerLogs(ip)
      setPeerLogs(logs)
    } catch (err) {
      console.error('Failed to get logs:', err)
      setPeerLogs([])
    } finally {
      setLogsLoading(false)
    }
  }

  const handleCloseLogsModal = () => {
    setShowLogsModal(null)
    setPeerLogs([])
    setLogsPage(0)
  }

  // Pagination helpers
  const totalLogsPages = Math.ceil(peerLogs.length / LOGS_PER_PAGE)
  const paginatedLogs = peerLogs.slice(logsPage * LOGS_PER_PAGE, (logsPage + 1) * LOGS_PER_PAGE)

  const handleAddPeer = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newPeerName.trim()) return

    setAddingPeer(true)
    try {
      await api.createPeer(newPeerName.trim())
      setNewPeerName('')
      setShowAddModal(false)
      await fetchPeers()
    } catch (err) {
      console.error('Failed to create peer:', err)
    } finally {
      setAddingPeer(false)
    }
  }

  const handleDeletePeer = (ip: string, name: string) => {
    setConfirmModal({
      show: true,
      title: 'Delete Client',
      message: `Are you sure you want to delete "${name}"? This action cannot be undone.`,
      onConfirm: async () => {
        try {
          await api.deletePeer(ip)
          await fetchPeers()
        } catch (err) {
          console.error('Failed to delete peer:', err)
        }
        setConfirmModal(null)
      }
    })
  }

  const handleToggleEnabled = async (peer: Peer) => {
    try {
      await api.updatePeer(peer.assigned_ip, { enabled: !peer.enabled })
      await fetchPeers()
    } catch (err) {
      console.error('Failed to toggle peer:', err)
    }
  }

  const handleSaveName = async (ip: string) => {
    if (!editName.trim()) return
    try {
      await api.updatePeer(ip, { name: editName.trim() })
      setEditingPeer(null)
      await fetchPeers()
    } catch (err) {
      console.error('Failed to update name:', err)
    }
  }

  const handleLogout = async () => {
    await api.logout()
    onLogout()
  }

  const handleDownloadConfig = async (ip: string, name: string) => {
    try {
      await api.downloadConfig(ip, `${name}.conf`)
    } catch (err) {
      console.error('Failed to download config:', err)
    }
  }

  const handleShowQRCode = async (ip: string) => {
    try {
      const url = await api.getQRCode(ip)
      setQrCodeUrl(url)
      setShowQRModal(ip)
    } catch (err) {
      console.error('Failed to get QR code:', err)
    }
  }

  const handleCloseQRModal = () => {
    setQrCodeUrl(null)
    setShowQRModal(null)
  }

  const onlineCount = peers.filter(p => p.is_online).length
  const totalTransfer = peers.reduce((acc, p) => acc + p.transfer_rx + p.transfer_tx, 0)

  return (
    <div className="dashboard">
      {/* Header */}
      <header className="header">
        <div className="header-brand">
          <div className="logo">
            <Shield size={20} />
          </div>
          <h1>WireGuard Server Panel</h1>
        </div>
        <div className="header-actions">
          <button onClick={fetchPeers} className="btn-icon" title="Refresh">
            <RefreshCw size={18} className={loading ? 'spin' : ''} />
          </button>
          <button onClick={onSettings} className="btn-icon" title="Settings">
            <Settings size={18} />
          </button>
          <button onClick={handleLogout} className="btn-icon" title="Logout">
            <LogOut size={18} />
          </button>
        </div>
      </header>

      {/* Stats Bar */}
      <div className="stats-bar">
        <div className="stat">
          <span className="stat-value">{peers.length}</span>
          <span className="stat-label">Clients</span>
        </div>
        <div className="stat">
          <span className="stat-value connected">{onlineCount}</span>
          <span className="stat-label">Connected</span>
        </div>
        <div className="stat">
          <span className="stat-value">{formatBytes(totalTransfer)}</span>
          <span className="stat-label">Total Transfer</span>
        </div>
      </div>

      {/* Client List */}
      <div className="client-list">
        {loading && peers.length === 0 ? (
          <div className="empty-state">
            <RefreshCw size={32} className="spin" />
            <p>Loading clients...</p>
          </div>
        ) : peers.length === 0 ? (
          <div className="empty-state">
            <Shield size={48} />
            <p>No clients configured</p>
          </div>
        ) : (
          peers.map(peer => (
            <div key={peer.id} className={`client-card ${!peer.enabled ? 'disabled' : ''}`}>
              <div className="client-status">
                <div className={`status-indicator ${peer.is_online ? 'online' : 'offline'}`} />
              </div>

              <div className="client-info">
                <div className="client-name-row">
                  {editingPeer === peer.assigned_ip ? (
                    <div className="edit-name">
                      <input
                        type="text"
                        value={editName}
                        onChange={e => setEditName(e.target.value)}
                        onKeyDown={e => {
                          if (e.key === 'Enter') handleSaveName(peer.assigned_ip)
                          if (e.key === 'Escape') setEditingPeer(null)
                        }}
                        autoFocus
                      />
                      <button onClick={() => handleSaveName(peer.assigned_ip)} className="btn-icon-sm">
                        <Check size={14} />
                      </button>
                      <button onClick={() => setEditingPeer(null)} className="btn-icon-sm">
                        <X size={14} />
                      </button>
                    </div>
                  ) : (
                    <>
                      <h3 className="client-name">{peer.name}</h3>
                      <button
                        onClick={() => {
                          setEditingPeer(peer.assigned_ip)
                          setEditName(peer.name)
                        }}
                        className="btn-icon-sm"
                        title="Edit name"
                      >
                        <Edit3 size={12} />
                      </button>
                    </>
                  )}
                </div>

                <div className="client-details">
                  <span className="detail">
                    <Globe size={12} />
                    {peer.assigned_ip}
                  </span>
                  <span className="detail">
                    <Clock size={12} />
                    {timeAgo(peer.latest_handshake)}
                  </span>
                </div>

                <div className="client-transfer">
                  <span className="transfer-item">
                    <ArrowDown size={12} />
                    {formatBytes(peer.transfer_rx)}
                  </span>
                  <span className="transfer-item">
                    <ArrowUp size={12} />
                    {formatBytes(peer.transfer_tx)}
                  </span>
                </div>
              </div>

              <div className="client-actions">
                <button
                  onClick={() => handleToggleEnabled(peer)}
                  className={`btn-icon-sm ${peer.enabled ? 'enabled' : 'disabled-btn'}`}
                  title={peer.enabled ? 'Disable' : 'Enable'}
                >
                  <Power size={16} />
                </button>
                <button
                  onClick={() => handleShowQRCode(peer.assigned_ip)}
                  className="btn-icon-sm"
                  title="Show QR Code"
                >
                  <QrCode size={16} />
                </button>
                {loggingEnabled && (
                  <button
                    onClick={() => handleShowLogs(peer.assigned_ip)}
                    className="btn-icon-sm"
                    title="View Logs"
                  >
                    <FileText size={16} />
                  </button>
                )}
                <button
                  onClick={() => handleDownloadConfig(peer.assigned_ip, peer.name)}
                  className="btn-icon-sm"
                  title="Download Config"
                >
                  <Download size={16} />
                </button>
                <button
                  onClick={() => handleDeletePeer(peer.assigned_ip, peer.name)}
                  className="btn-icon-sm delete"
                  title="Delete"
                >
                  <Trash2 size={16} />
                </button>
              </div>
            </div>
          ))
        )}
      </div>

      {/* Floating Add Button */}
      <button onClick={() => setShowAddModal(true)} className="fab">
        <Plus size={24} />
      </button>

      {/* Add Client Modal */}
      {showAddModal && (
        <div className="modal-overlay" onClick={() => setShowAddModal(false)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <div className="modal-header">
              <h2>New Client</h2>
              <button onClick={() => setShowAddModal(false)} className="btn-icon-sm">
                <X size={18} />
              </button>
            </div>
            <form onSubmit={handleAddPeer}>
              <div className="modal-body">
                <label htmlFor="clientName">Client Name</label>
                <input
                  id="clientName"
                  type="text"
                  value={newPeerName}
                  onChange={e => setNewPeerName(e.target.value)}
                  placeholder="e.g., My Phone"
                  required
                  autoFocus
                />
              </div>
              <div className="modal-footer">
                <button type="button" onClick={() => setShowAddModal(false)} className="btn-secondary">
                  Cancel
                </button>
                <button type="submit" className="btn-primary" disabled={addingPeer}>
                  {addingPeer ? 'Creating...' : 'Create'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* QR Code Modal */}
      {showQRModal && qrCodeUrl && (
        <div className="modal-overlay" onClick={handleCloseQRModal}>
          <div className="modal qr-modal" onClick={e => e.stopPropagation()}>
            <div className="modal-header">
              <h2>Scan QR Code</h2>
              <button onClick={handleCloseQRModal} className="btn-icon-sm">
                <X size={18} />
              </button>
            </div>
            <div className="modal-body qr-body">
              <img
                src={qrCodeUrl}
                alt="WireGuard QR Code"
                className="qr-image"
              />
              <p>Scan with WireGuard app</p>
            </div>
          </div>
        </div>
      )}

      {/* Logs Modal */}
      {showLogsModal && (
        <div className="modal-overlay" onClick={handleCloseLogsModal}>
          <div className="modal logs-modal" onClick={e => e.stopPropagation()}>
            <div className="modal-header">
              <h2>Connection Logs</h2>
              <button onClick={handleCloseLogsModal} className="btn-icon-sm">
                <X size={18} />
              </button>
            </div>
            <div className="modal-body logs-body">
              {logsLoading ? (
                <div className="logs-loading">
                  <RefreshCw size={24} className="spin" />
                  <p>Loading logs...</p>
                </div>
              ) : peerLogs.length === 0 ? (
                <div className="logs-empty">
                  <FileText size={32} />
                  <p>No connection logs yet</p>
                </div>
              ) : (
                <>
                  <div className="logs-list">
                    {paginatedLogs.map(log => (
                      <div key={log.id} className="log-entry">
                        <span className="log-endpoint">{log.endpoint}</span>
                        <span className="log-time">
                          {new Date(log.connected_at).toLocaleString()}
                        </span>
                      </div>
                    ))}
                  </div>
                  {totalLogsPages > 1 && (
                    <div className="logs-pagination">
                      <button
                        onClick={() => setLogsPage(p => Math.max(0, p - 1))}
                        disabled={logsPage === 0}
                        className="btn-icon-sm"
                      >
                        <ChevronLeft size={16} />
                      </button>
                      <span className="page-info">
                        {logsPage + 1} / {totalLogsPages}
                      </span>
                      <button
                        onClick={() => setLogsPage(p => Math.min(totalLogsPages - 1, p + 1))}
                        disabled={logsPage >= totalLogsPages - 1}
                        className="btn-icon-sm"
                      >
                        <ChevronRight size={16} />
                      </button>
                    </div>
                  )}
                </>
              )}
            </div>
          </div>
        </div>
      )}

      {/* Confirm Modal */}
      {confirmModal?.show && (
        <div className="modal-overlay" onClick={() => setConfirmModal(null)}>
          <div className="modal confirm-modal" onClick={e => e.stopPropagation()}>
            <div className="modal-header">
              <h2>{confirmModal.title}</h2>
              <button onClick={() => setConfirmModal(null)} className="btn-icon-sm">
                <X size={18} />
              </button>
            </div>
            <div className="modal-body">
              <p className="confirm-message">{confirmModal.message}</p>
            </div>
            <div className="modal-footer">
              <button type="button" onClick={() => setConfirmModal(null)} className="btn-secondary">
                Cancel
              </button>
              <button type="button" onClick={confirmModal.onConfirm} className="btn-danger">
                Delete
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Footer */}
      <footer className="app-footer">
        <a href="https://www.layerweb.com.tr" target="_blank" rel="noopener noreferrer">
          Powered by <strong>LayerWeb</strong>
        </a>
      </footer>
    </div>
  )
}
