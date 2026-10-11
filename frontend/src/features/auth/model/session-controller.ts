import { initialSessionState, type SessionState, type SessionSnapshot } from '@/entities/session'
import { isApiError, type ApiError } from '@/shared/api'
import type { AuthAdapter, LoginCredentials } from '../api/draft-auth-adapter'

interface SessionPorts {
  read: () => SessionState
  write: (state: SessionState) => void
  setCsrf: (value: string | null) => void
  clearApi: () => void
}
const interrupted: ApiError = { status: 401, code: 'SESSION_CHANGED', message: 'Сессия изменилась. Повторите вход.', details: {} }

export function createSessionController(adapter: AuthAdapter, ports: SessionPorts) {
  let version = 0
  let pending: Promise<boolean> | null = null
  let active: AbortController | null = null
  let csrf: string | null = null
  const subscriptions = new Set<() => void>()
  function cleanup() {
    csrf = null
    ports.setCsrf(null)
    active?.abort()
    active = null
    ports.clearApi()
    for (const close of subscriptions) {
      try { close() } catch { /* A failed disposer must not prevent other cleanup. */ }
    }
    subscriptions.clear()
  }
  function accept(snapshot: SessionSnapshot) {
    const previous = ports.read()
    csrf = snapshot.csrfToken
    ports.setCsrf(csrf)
    ports.write({ ...initialSessionState, status: 'authenticated', user: snapshot.user, settings: snapshot.settings, expiresAt: snapshot.expiresAt,
      draftOwner: snapshot.user.id, drafts: previous.draftOwner === snapshot.user.id ? previous.drafts : {},
    })
  }
  function expire() {
    if (ports.read().status !== 'authenticated') return
    version++
    pending = null
    const { draftOwner, drafts } = ports.read()
    cleanup()
    ports.write({ ...initialSessionState, status: 'anonymous', draftOwner, drafts, notice: 'Сессия истекла. Войдите снова, чтобы продолжить работу.' })
  }
  function restore(): Promise<boolean> {
    if (pending) return pending
    const state = ports.read()
    if (state.status === 'authenticated') {
      if (state.expiresAt && Date.parse(state.expiresAt) <= Date.now()) { expire(); return Promise.resolve(false) }
      return Promise.resolve(true)
    }
    if (state.status === 'anonymous' || state.status === 'signing-out') return Promise.resolve(false)
    const attempt = ++version
    active = new AbortController()
    ports.write({ ...state, status: 'restoring', notice: null })
    pending = adapter.me(active.signal).then((snapshot) => {
      if (attempt !== version) return false
      accept(snapshot)
      return true
    }).catch((error: unknown) => {
      if (attempt !== version) return false
      const anonymous = isApiError(error) && (error.status === 401 || error.status === 403)
      cleanup()
      ports.write({ ...initialSessionState, status: anonymous ? 'anonymous' : 'error',
        notice: anonymous ? null : 'Не удалось проверить сессию. Повторите проверку перед продолжением.',
      })
      return false
    }).finally(() => { if (attempt === version) { pending = null; active = null } })
    return pending
  }
  async function login(credentials: LoginCredentials) {
    if (pending || active || ['signing-out', 'authenticated'].includes(ports.read().status)) throw interrupted
    const attempt = ++version
    active = new AbortController()
    try {
      const snapshot = await adapter.login(credentials, active.signal)
      if (attempt !== version) throw interrupted
      accept(snapshot)
    } finally { if (attempt === version) active = null }
  }
  async function logout() {
    const attempt = ++version
    const token = csrf
    cleanup()
    pending = null
    ports.write({ ...initialSessionState, status: 'signing-out' })
    active = new AbortController()
    let failed = false
    try { await adapter.logout(token, active.signal) } catch { failed = true }
    finally {
      if (attempt === version) {
        active = null
        ports.write({ ...initialSessionState, status: 'anonymous', notice: failed
          ? 'Данные на устройстве очищены. Сервер не подтвердил выход; закройте приложение и повторите выход после восстановления связи.' : null })
      }
    }
  }
  return {
    restore, login, logout, expire,
    getVersion: () => version,
    hasAccess: () => { const state = ports.read(); return state.status === 'authenticated' && !!state.expiresAt && Date.parse(state.expiresAt) > Date.now() },
    registerSubscription: (close: () => void) => { subscriptions.add(close); return () => subscriptions.delete(close) },
  }
}
export type SessionController = ReturnType<typeof createSessionController>
