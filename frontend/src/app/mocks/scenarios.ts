export const mockScenarios = [
  { value: 'supply-not-found', label: 'Поставка · Не найдена' },
  { value: 'supplies-empty', label: 'Поставки · Пустая история' },
  { value: 'supplies-received', label: 'Поставки · После тестовой приёмки' },
  { value: 'inventory-empty', label: 'Склад · Пустой список' },
  { value: 'batches-empty', label: 'Партии · Пустой список' },
  { value: 'batches-error', label: 'Партии · Ошибка панели' },
  { value: 'batches-slow', label: 'Партии · Медленная загрузка' },
  { value: 'categories-error', label: 'Категории · Ошибка загрузки' },
  { value: 'role-admin', label: 'Роль · Администратор' },
  { value: 'role-storekeeper', label: 'Роль · Кладовщик' },
  { value: 'role-order-operator', label: 'Роль · Оператор заказов' },
  { value: 'invalid-password', label: 'Вход · Неверный пароль' },
  { value: 'disabled-user', label: 'Вход · Пользователь отключён' },
  { value: 'session-expired', label: 'Сессия истекла' },
  { value: 'success', label: 'Успешный ответ' },
  { value: 'slow', label: 'Медленный ответ · 2 секунды' },
  { value: 'network-error', label: 'Обрыв сети' },
  { value: 'unauthorized', label: '401 · Нет сессии' },
  { value: 'forbidden', label: '403 · Нет доступа' },
  { value: 'conflict', label: '409 · Конфликт' },
  { value: 'validation-error', label: '422 · Ошибка полей' },
  { value: 'rate-limit', label: '429 · Лимит запросов' },
  { value: 'server-error', label: '500 · Ошибка сервера' },
  { value: 'bad-gateway', label: '502 · HTML от прокси' },
  { value: 'invalid-json', label: '200 · Некорректный JSON' },
  { value: 'invalid-schema', label: '200 · Неверная структура' },
] as const

export type MockScenario = typeof mockScenarios[number]['value']

let currentScenario: MockScenario = 'success'

export function isMockScenario(value: string): value is MockScenario {
  return mockScenarios.some((scenario) => scenario.value === value)
}

export function setMockScenario(value: string) {
  if (!isMockScenario(value)) throw new Error(`Неизвестный сценарий моков: ${value}`)
  currentScenario = value
}

export function getMockScenario(): MockScenario {
  return currentScenario
}
