import { useEffect, useRef, ReactNode } from 'react'
import { Icon } from './Icon'

interface ModalProps {
  title: string
  icon?: string
  danger?: boolean
  wide?: boolean
  onClose: () => void
  children: ReactNode
  footer?: ReactNode
}

/** Native <dialog> styled by the theme (focus trap, Esc and backdrop for free). */
export default function Modal({ title, icon = 'info', danger, wide, onClose, children, footer }: ModalProps) {
  const ref = useRef<HTMLDialogElement>(null)

  useEffect(() => {
    const dlg = ref.current
    if (dlg && !dlg.open) dlg.showModal()
    return () => {
      if (dlg?.open) dlg.close()
    }
  }, [])

  return (
    <dialog
      ref={ref}
      className={wide ? 'modal modal-wide' : 'modal'}
      onClose={onClose}
      onClick={e => {
        if (e.target === ref.current) onClose()
      }}
      aria-label={title}
    >
      <div className={danger ? 'modal-h danger' : 'modal-h'}>
        <Icon name={icon} />
        <h2>{title}</h2>
      </div>
      <div className="modal-b">{children}</div>
      {footer && <div className="modal-f">{footer}</div>}
    </dialog>
  )
}
