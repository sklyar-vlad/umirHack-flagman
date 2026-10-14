import { useEffect, useEffectEvent, useId, useRef, type ReactNode } from 'react'
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
  returnFocusRef?: { current: HTMLElement | null }
}

function Modal({ open, onClose, title, description, children, actions, closeLabel = 'Закрыть', initialFocusRef, returnFocusRef, drawer = false }: DialogProps & { drawer?: boolean }) {
  const ref = useRef<HTMLDialogElement>(null)
  const id = useId()
  const focusTargets = useEffectEvent(() => ({ initial: initialFocusRef?.current, previous: returnFocusRef?.current ?? document.activeElement }))
  useEffect(() => {
    const dialog = ref.current
    if (!dialog || !open) return
    const { initial, previous: previousFocus } = focusTargets()
    dialog.showModal()
    initial?.focus()
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      dialog.close()
      document.body.style.overflow = previousOverflow
      if (previousFocus instanceof HTMLElement && previousFocus.isConnected) previousFocus.focus()
    }
  }, [open])

  return <dialog ref={ref} className={`ui-dialog${drawer ? ' ui-drawer' : ''}`} aria-labelledby={`${id}-title`} aria-describedby={description ? `${id}-description` : undefined}
    onKeyDown={(event) => {
      if (event.key !== 'Tab' || event.defaultPrevented) return
      const dialog = event.currentTarget
      if (document.activeElement?.closest('dialog') !== dialog) return
      const controls = [...dialog.querySelectorAll<HTMLElement>('button, a[href], input, select, textarea, [tabindex]')]
        .filter((element) => element.tabIndex >= 0 && !element.matches(':disabled') && element.getClientRects().length > 0 && element.closest('dialog') === dialog)
      const first = controls[0], last = controls.at(-1)
      if (first && last && (event.shiftKey && document.activeElement === first || !event.shiftKey && document.activeElement === last)) {
        event.preventDefault(); event.stopPropagation(); (event.shiftKey ? last : first).focus()
      }
    }}
    onCancel={(event) => { event.preventDefault(); event.stopPropagation(); onClose() }}>
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
