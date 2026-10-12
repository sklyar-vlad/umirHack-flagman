import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useSelector } from 'react-redux'
import { canAccess, sectionForPath, selectSession } from '@/entities/session'
import type { WriteOffDraft } from '@/shared/api/write-off-work'
import { Button } from '@/shared/ui'
import { WriteOffDialog } from './WriteOffDialog'
import { WriteOffContext, type OpenWriteOff } from './write-off-context'

/** Lives outside refreshed batch rows, retaining the pending request and result. */
export function WriteOffWorkspace({ children }: { children: ReactNode }) {
  const session = useSelector(selectSession)
  return <OwnerWorkspace key={session.user?.id ?? 'anonymous'} session={session}>{children}</OwnerWorkspace>
}

function OwnerWorkspace({ children, session }: { children: ReactNode; session: ReturnType<typeof selectSession> }) {
  const saved = session.draftOwner === session.user?.id ? session.drafts.writeOff?.writeOff : undefined
  const [selected, setSelected] = useState<WriteOffDraft | undefined>(saved)
  const focus = useRef<HTMLElement | null>(null), root = useRef<HTMLDivElement>(null)
  useEffect(() => { if (!selected) focus.current?.focus() }, [selected])
  const allowed = !!session.user && canAccess(session.user.role, sectionForPath('/write-offs')!)
  const open: OpenWriteOff = (snapshot, trigger) => {
    if (!allowed) return
    focus.current = trigger
    setSelected(saved?.attempt ? saved : { ...snapshot, values: { quantity: '', reason: '' } })
  }
  return <WriteOffContext value={open}>
    <div ref={root} tabIndex={-1} aria-label="Работа со складом">
      {saved && !selected && allowed && <p role="status">{saved.attempt ? 'Есть запрос списания с неизвестным результатом.' : 'Сохранён черновик списания.'} <Button variant="secondary" onClick={(event) => { focus.current = event.currentTarget; setSelected(saved) }}>Продолжить списание</Button></p>}
      {children}
    </div>
    {selected && allowed && <WriteOffDialog key={selected.batch.id} initial={selected} returnFocusRef={focus} onClose={() => {
      if (!focus.current?.isConnected) focus.current = root.current?.querySelector<HTMLButtonElement>(`[data-writeoff-batch="${selected.batch.id}"]`) ?? root.current
      setSelected(undefined)
    }} />}
  </WriteOffContext>
}
