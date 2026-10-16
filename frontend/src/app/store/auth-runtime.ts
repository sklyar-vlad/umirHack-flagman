import { persistWriteOff } from './write-off-persistence'
import { persistDishes } from './dish-persistence'
import { replaceSession } from '@/entities/session'
import { createSessionController, draftAuthAdapter } from '@/features/auth'
import { baseApi } from '@/shared/api'
import { store } from './store'
import { persistProducts } from './product-persistence'
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

if (typeof window !== 'undefined') {
  try { persistWriteOff(store, window.sessionStorage); persistReception(store, window.sessionStorage); persistOrder(store, window.sessionStorage); persistProducts(store, window.sessionStorage); persistDishes(store, window.sessionStorage) } catch {  }
}
