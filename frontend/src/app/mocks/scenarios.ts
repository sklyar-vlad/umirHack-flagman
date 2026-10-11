export const mockScenarios = [
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
