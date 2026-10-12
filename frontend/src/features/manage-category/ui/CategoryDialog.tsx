import { useRef, type ReactNode } from 'react'
import type { Category } from '@/entities/product-category'
import { Dialog } from '@/shared/ui'
import { CategoryForm, type CategoryFormHandle } from './CategoryForm'
import { CategoryEditor } from './CategoryEditor'

// Mount only while open, so every editor starts with its own fresh revision.
export function CategoryDialog({ id, onSaved, onClose, onDirty, returnFocusRef, actions }: {
  id?: string
  actions?: ReactNode
  returnFocusRef?: { current: HTMLElement | null }
  onSaved: (category: Category) => void
  onClose: () => void
  onDirty: (dirty: boolean) => void
}) {
  const form = useRef<CategoryFormHandle>(null), input = useRef<HTMLInputElement>(null)
  const close = () => { if (form.current) form.current.requestClose(); else onClose() }
  return <Dialog open initialFocusRef={input} actions={actions} returnFocusRef={returnFocusRef} title={id ? 'Изменение категории' : 'Создание категории'} onClose={close}>
    {id ? <CategoryEditor ref={form} focusRef={input} id={id} onSaved={onSaved} onCancel={onClose} onDirty={onDirty} /> : <CategoryForm ref={form} focusRef={input} onSaved={onSaved} onCancel={onClose} onDirty={onDirty} onReload={async () => undefined} />}
  </Dialog>
}
