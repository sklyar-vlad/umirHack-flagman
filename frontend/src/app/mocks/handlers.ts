import { forecastHandlers } from './forecast-handlers'
import { eventHandlers } from './event-handlers'
import { shortageHandlers } from './shortage-handlers'
import { writeOffHandlers } from './write-off-handlers'
import { lossHandlers } from './loss-handlers'
import { movementHandlers } from './movement-handlers'
import { auditHandlers } from './audit-handlers'
import { userHandlers } from './user-handlers'
import { settingsHandlers } from './settings-handlers'
import { dishEditorHandlers } from './dish-editor-handlers'
import { catalogHandlers } from './catalog-handlers'
import { dashboardHandlers } from './dashboard-handlers'
import { hotOfferHandlers } from './hot-offer-handlers'
import { procurementHandlers } from './procurement-handlers'
import { orderCompositionHandlers } from './order-composition-handlers'
import { orderHandlers } from './order-handlers'
import { stopListHandlers } from './stop-list-handlers'
import { receptionHandlers } from './reception-handlers'
import { authHandlers } from './auth-handlers'
import { inventoryHandlers } from './inventory-handlers'
import { supplyHandlers } from './supply-handlers'
import { delay } from 'msw'
import { http } from 'msw/http'
import { createHealthcheckResponse, createMockError } from './responses'
import { getMockScenario } from './scenarios'

function missingHandler() {
  return createMockError(501, 'MOCK_NOT_IMPLEMENTED', 'Для этого API-маршрута мок ещё не реализован')
}

export const handlers = [
  ...eventHandlers,
  ...authHandlers,
  ...writeOffHandlers,
  ...auditHandlers,
  ...movementHandlers,
  ...lossHandlers,
  ...shortageHandlers,
  ...forecastHandlers,
  ...settingsHandlers,
  ...userHandlers,
  ...dashboardHandlers,
  ...orderHandlers,
  ...stopListHandlers,
  ...procurementHandlers,
  ...hotOfferHandlers,
  ...dishEditorHandlers,
  ...orderCompositionHandlers,
  ...catalogHandlers,
  ...inventoryHandlers,
  ...supplyHandlers,
  ...receptionHandlers,
  http.get('/api/v1/healthcheck', async () => {
    // Фиксируем сценарий для конкретной попытки до задержки.
    const scenario = getMockScenario()
    if (scenario === 'slow') await delay(2_000)
    return createHealthcheckResponse(scenario)
  }),
  // В моковом режиме неизвестный API не обращается к реальному складу.
  // Предметные handlers добавляются выше по мере реализации экранов.
  http.all('/api/v1/*', missingHandler),
  http.all('/api/v1', missingHandler),
]
