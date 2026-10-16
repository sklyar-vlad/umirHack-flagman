import { createListenerMiddleware, isAnyOf } from '@reduxjs/toolkit'
import { dishReadsChanged } from '@/entities/dish'
import { orderApi } from '@/entities/order'
import { supplyApi } from '@/entities/supply'
import { writeOffApi } from '@/entities/stock-movement'
import { productApi } from '@/entities/product'
import { categoryApi } from '@/entities/product-category'
import { settingsApi } from '@/entities/restaurant-settings'
import type { SessionState } from '@/entities/session'

export const liveMutations = createListenerMiddleware<{ session: SessionState }>()
const writes = [supplyApi.endpoints.receiveSupply, orderApi.endpoints.createOrder, writeOffApi.endpoints.writeOff, productApi.endpoints.createProduct, productApi.endpoints.updateProduct, categoryApi.endpoints.createCategory, categoryApi.endpoints.updateCategory, settingsApi.endpoints.updateSettings] as const
liveMutations.startListening({
  matcher: isAnyOf(...writes.flatMap((endpoint) => [endpoint.matchFulfilled, endpoint.matchRejected])),
  effect: (_action, api) => { if (api.getState().session.status === 'authenticated') api.dispatch(dishReadsChanged()) },
})
