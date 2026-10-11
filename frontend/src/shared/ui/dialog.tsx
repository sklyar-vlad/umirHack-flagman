import { useEffect, useId, useRef, type ReactNode } from 'react'
import { Button, Icon } from './controls'

export type DialogProps = {
  open: boolean
  onClose: () => void
  title: string
  description?: string
  children: ReactNode
  actions?: ReactNode
  closeLabel?: string
  initialFocusRef?: { current: HTMLElement | null }
}

function Modal({ open, onClose, title, description, children, actions, closeLabel = 'Закрыть', initialFocusRef, drawer = false }: DialogProps & { drawer?: boolean }) {
  const ref = useRef<HTMLDialogElement>(null)
  const id = useId()
  useEffect(() => {
    const dialog = ref.current
    if (!dialog || !open) return
    const previousFocus = document.activeElement
    dialog.showModal()
    initialFocusRef?.current?.focus()
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      dialog.close()
      document.body.style.overflow = previousOverflow
      if (previousFocus instanceof HTMLElement && previousFocus.isConnected) previousFocus.focus()
    }
  }, [open, initialFocusRef])

  return <dialog ref={ref} className={`ui-dialog${drawer ? ' ui-drawer' : ''}`} aria-labelledby={`${id}-title`} aria-describedby={description ? `${id}-description` : undefined}
    onCancel={(event) => { event.preventDefault(); onClose() }}>
    <div className="ui-dialog-header"><h2 id={`${id}-title`}>{title}</h2><Button variant="ghost" aria-label={closeLabel} onClick={onClose}><Icon name="close" /></Button></div>
    {description && <p id={`${id}-description`} className="text-muted">{description}</p>}
    <div className="ui-dialog-content">{children}</div>
    {actions && <div className="ui-actions">{actions}</div>}
  </dialog>
}

export function Dialog(props: DialogProps) {
  return <Modal {...props} />
}

export function Drawer(props: DialogProps) {
  return <Modal {...props} drawer />
}
