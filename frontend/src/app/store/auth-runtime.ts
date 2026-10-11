import { replaceSession } from '@/entities/session'
import { createSessionController, draftAuthAdapter } from '@/features/auth'
import { baseApi } from '@/shared/api'
import { store } from './store'
import { persistOrder } from './order-persistence'
import { persistReception } from './reception-persistence'
import { apiTransport, setCsrfToken } from './session-transport'

export const authController = createSessionController(draftAuthAdapter, {
  read: () => store.getState().session,
  write: (state) => { store.dispatch(replaceSession(state)) },
  setCsrf: setCsrfToken,
  clearApi: () => {
    for (const request of store.dispatch(baseApi.util.getRunningQueriesThunk())) request.abort()
    for (const request of store.dispatch(baseApi.util.getRunningMutationsThunk())) request.abort()
    store.dispatch(baseApi.util.resetApiState())
  },
})
apiTransport.waitForSession = authController.restore
apiTransport.onUnauthorized = authController.expire
apiTransport.getSessionVersion = authController.getVersion
apiTransport.hasSessionAccess = authController.hasAccess

// Only the reception allowlist is persisted; session tokens and CSRF stay in memory.
if (typeof window !== 'undefined') {
  try { persistReception(store, window.sessionStorage); persistOrder(store, window.sessionStorage) } catch { /* Browser storage disabled. */ }
}
