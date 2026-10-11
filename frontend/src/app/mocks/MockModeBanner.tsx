import { useState } from 'react'
import { baseApi } from '@/shared/api'
import { inventoryApi } from '@/entities/inventory'
import { useAppDispatch } from '../store'
import { getMockScenario, mockScenarios, setMockScenario, type MockScenario } from './scenarios'
import styles from './MockModeBanner.module.css'

export function MockModeBanner() {
  const [scenario, setScenario] = useState<MockScenario>(getMockScenario)
  const dispatch = useAppDispatch()
  const { isFetching } = baseApi.endpoints.healthcheck.useQueryState()

  function changeScenario(value: string) {
    setMockScenario(value)
    setScenario(getMockScenario())
    // Обновляются только активные подписки, последний ответ не очищается.
    dispatch(baseApi.util.invalidateTags(['Healthcheck']))
    dispatch(inventoryApi.util.invalidateTags(['Inventory', 'Batches', 'ProductCategories']))
  }

  return (
    <aside className={styles.banner} aria-label="Настройки мокового режима">
      <div className={styles.description}>
        <p className="text-strong">Моковый режим</p>
        <p className="text-caption">API заменён тестовыми ответами. Это не проверка backend.</p>
      </div>
      <label className={styles.field} htmlFor="mock-scenario">
        <span className="text-label">Сценарий API</span>
        <select
          className="ui-select"
          id="mock-scenario"
          value={scenario}
          disabled={isFetching}
          onChange={(event) => changeScenario(event.target.value)}
        >
          {mockScenarios.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}
        </select>
      </label>
      <p className="text-caption">Для реального API отключите VITE_ENABLE_MOCKS и перезапустите Vite.</p>
    </aside>
  )
}
