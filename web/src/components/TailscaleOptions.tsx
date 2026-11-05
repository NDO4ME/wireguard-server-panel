import { useState, useEffect, useCallback } from 'react'
import { api } from '../api/client'
import type { TailscalePrefs } from '../api/client'
import { Icon } from './Icon'

// Runtime Tailscale options, applied with `tailscale set` (no reconnect needed).
export default function TailscaleOptions() {
  const [prefs, setPrefs] = useState<TailscalePrefs | null>(null)
  const [routes, setRoutes] = useState('')
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useState<{ kind: 'ok' | 'err'; text: string } | null>(null)

  const load = useCallback(async () => {
    try {
      const p = await api.getTailscalePrefs()
      setPrefs(p)
      setRoutes((p.advertise_routes || []).join('\n'))
    } catch (err) {
      setMsg({ kind: 'err', text: err instanceof Error ? err.message : 'Could not read Tailscale options.' })
    }
  }, [])

  useEffect(() => { load() }, [load])

  if (!prefs) {
    return msg ? <div className="msg msg-err compact" role="alert"><Icon name="x-circle" /><div className="msg-body">{msg.text}</div></div> : null
  }

  const parsed = routes.split(/[\s,]+/).filter(Boolean)
  const dirty =
    JSON.stringify(parsed) !== JSON.stringify(prefs.advertise_routes || [])

  const save = async (next: TailscalePrefs) => {
    setSaving(true)
    setMsg(null)
    try {
      const p = await api.setTailscalePrefs(next)
      setPrefs(p)
      setRoutes((p.advertise_routes || []).join('\n'))
      setMsg({ kind: 'ok', text: 'Applied. Advertised routes and exit nodes may need approval in the Tailscale admin console.' })
    } catch (err) {
      setMsg({ kind: 'err', text: err instanceof Error ? err.message : 'Failed to apply options.' })
    } finally {
      setSaving(false)
    }
  }

  const toggle = (k: 'advertise_exit_node' | 'shields_up' | 'accept_routes') => (e: React.ChangeEvent<HTMLInputElement>) =>
    save({ ...prefs, [k]: e.target.checked })

  return (
    <section className="panel">
      <header className="panel-h"><h2><Icon name="sliders" />Node options</h2></header>
      <div className="panel-b">
        {msg && (
          <div className={`msg ${msg.kind === 'ok' ? 'msg-ok' : 'msg-err'} compact`} role={msg.kind === 'ok' ? 'status' : 'alert'}>
            <Icon name={msg.kind === 'ok' ? 'check-circle' : 'x-circle'} /><div className="msg-body">{msg.text}</div>
          </div>
        )}
        <label className="check">
          <input type="checkbox" checked={prefs.accept_routes} disabled={saving} onChange={toggle('accept_routes')} />
          <span>Accept routes from other nodes<span className="hint">Reach subnets advertised elsewhere in your tailnet.</span></span>
        </label>
        <label className="check">
          <input type="checkbox" checked={prefs.advertise_exit_node} disabled={saving} onChange={toggle('advertise_exit_node')} />
          <span>Advertise as exit node<span className="hint">Lets tailnet devices send all their traffic through this server. Approve it in the admin console.</span></span>
        </label>
        <label className="check">
          <input type="checkbox" checked={prefs.shields_up} disabled={saving} onChange={toggle('shields_up')} />
          <span>Shields up<span className="hint">Blocks all incoming connections from the tailnet to this node. Outgoing and routed traffic keep working.</span></span>
        </label>
        <label className="field">
          <span className="lbl">Advertised subnet routes <span className="opt">one CIDR per line</span></span>
          <textarea rows={3} value={routes} disabled={saving} onChange={e => setRoutes(e.target.value)} placeholder="192.168.1.0/24" spellCheck={false} />
          <span className="hint">Share networks reachable from this server with your tailnet.</span>
        </label>
        <div className="form-actions">
          <button type="button" className="btn btn-primary" disabled={saving || !dirty} onClick={() => save({ ...prefs, advertise_routes: parsed })}>
            <Icon name="check" />{saving ? 'Applying…' : 'Apply routes'}
          </button>
        </div>
      </div>
    </section>
  )
}
