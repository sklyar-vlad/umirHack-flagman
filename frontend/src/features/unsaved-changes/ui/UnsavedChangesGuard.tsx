import { useEffect, useRef } from 'react'
import { useBlocker } from 'react-router'
import { Button, Dialog } from '@/shared/ui'

export function UnsavedChangesGuard({ dirty, pending = false, onDiscard }: { dirty: boolean; pending?: boolean; onDiscard?: () => void }) {
  const blocker = useBlocker(dirty)
  const stayRef = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    if (!dirty) return
    const beforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', beforeUnload)
    return () => window.removeEventListener('beforeunload', beforeUnload)
  }, [dirty])
  useEffect(() => {
    if (!dirty && blocker.state === 'blocked') blocker.proceed()
  }, [dirty, blocker])
  return <Dialog open={blocker.state === 'blocked'} title="Уйти без сохранения?" description={pending ? "Дождитесь результата сохранения перед уходом." : "Изменения в форме будут потеряны."} initialFocusRef={stayRef}
    onClose={() => { if (blocker.state === 'blocked') blocker.reset() }}
    actions={<><Button ref={stayRef} variant="secondary" onClick={() => { if (blocker.state === 'blocked') blocker.reset() }}>Остаться</Button><Button variant="danger" disabled={pending} onClick={() => { if (blocker.state === 'blocked' && !pending) { onDiscard?.(); blocker.proceed() } }}>Уйти без сохранения</Button></>}>
    <p>Сохраните форму перед переходом, если хотите сохранить изменения.</p>
  </Dialog>
}
