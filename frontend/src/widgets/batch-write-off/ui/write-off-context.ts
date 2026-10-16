import { createContext, useContext } from 'react'
import type { WriteOffDraft } from '@/shared/api/write-off-work'
export type OpenWriteOff = (snapshot: Pick<WriteOffDraft, 'batch' | 'product'>, trigger: HTMLButtonElement) => void
export const WriteOffContext = createContext<OpenWriteOff>(() => {})
export const useWriteOff = () => useContext(WriteOffContext)
