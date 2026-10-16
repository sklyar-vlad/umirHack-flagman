import { dishReadsChanged } from '@/entities/dish'
import { createLiveEvents } from '@/shared/api/live-events'
import type { SessionController } from '@/features/auth'
import type { AppStore } from './store'
import { liveApi, liveTags } from './live-tags'
import { expiryBoundaryCrossed } from './expiry-refresh'

export function startLiveRuntime(store: AppStore, auth: SessionController) {
  let key = '', dispose: (() => void) | undefined
  function sync() {
    const session = store.getState().session
    const next = session.status === 'authenticated' && session.user ? `${auth.getVersion()}:${session.user.id}:${session.user.role}` : ''
    if (next === key) return
    dispose?.(); dispose = undefined; key = next
    if (!next) return
    let lastClock = 0
    const live = createLiveEvents({
      connect: () => new EventSource('/api/v1/events', { withCredentials: true }),
      visible: () => document.visibilityState === 'visible', online: () => navigator.onLine,
      confirmSession: auth.revalidate,
      refresh: (events, full) => {
        const current = store.getState().session
        if (!auth.hasAccess() || !current.user || key !== next) return
        const updates = events.filter((event) => {
          if (event.type !== 'inventory.time_changed') return true
          const now = Date.parse(event.occurred_at)
          const due = Object.values(store.getState().api.queries).some((query) => expiryBoundaryCrossed(query?.data, now, lastClock, current.settings?.expiringThresholdHours ?? 24))
          lastClock = Math.max(lastClock, now)
          return due
        })
        if (!full && !updates.length) return
        store.dispatch(liveApi.util.invalidateTags(liveTags(current.user.role, updates, full)))
        if (full || updates.some((event) => event.topics.some((topic) => ['inventory', 'batches', 'dishes', 'products', 'settings'].includes(topic)))) store.dispatch(dishReadsChanged())
      },
    })
    const wake = () => live.wake()
    window.addEventListener('online', wake); window.addEventListener('offline', wake); document.addEventListener('visibilitychange', wake)
    const unregister = auth.registerSubscription(live.stop)
    dispose = () => {
      live.stop(); unregister()
      window.removeEventListener('online', wake); window.removeEventListener('offline', wake); document.removeEventListener('visibilitychange', wake)
    }
  }
  const unsubscribe = store.subscribe(sync)
  sync()
  return () => { unsubscribe(); dispose?.() }
}
