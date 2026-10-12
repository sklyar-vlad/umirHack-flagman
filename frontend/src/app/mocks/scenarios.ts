export const mockScenarios = [
  { value: 'dish-catalog-new-version', label: 'Блюда · Новая версия рецептуры' },
  { value: 'product-save-slow', label: 'Продукт · Сохранение' },
  { value: 'product-lost-response', label: 'Продукт · Ответ потерян после записи' },
  { value: 'product-revision-conflict', label: 'Продукт · Конфликт ревизии' },
  { value: 'product-validation-error', label: 'Продукт · Поля 422' },
  { value: 'product-unit-locked', label: 'Продукт · UNIT_LOCKED' },
  { value: 'product-in-use', label: 'Продукт · PRODUCT_IN_USE' },
  { value: 'product-not-found', label: 'Продукт · Не найден' },

  { value: 'category-validation-error', label: 'Категории · Отказ 422' },
  { value: 'category-not-found', label: 'Категории · Отказ 404 при изменении' },
  { value: 'categories-empty', label: 'Категории · Пустой каталог' },
  { value: 'category-save-slow', label: 'Категории · Сохранение' },
  { value: 'category-revision-conflict', label: 'Категории · Конфликт ревизии' },
  { value: 'category-lost-response', label: 'Категории · Ответ потерян после записи' },
  { value: 'dashboard-empty', label: 'Обзор · Пустые блоки' },
  { value: 'dashboard-preview', label: 'Обзор · Больше 20 партий' },
  { value: 'dashboard-expired', label: 'Обзор · Партия истекла' },
  { value: 'hot-offers-empty', label: 'Предложения · Нет рекомендаций' },
  { value: 'hot-offers-expired', label: 'Предложения · Партия истекла' },
  { value: 'hot-offers-expiring', label: 'Предложения · Срок через секунду' },
  { value: 'hot-offers-high', label: 'Предложения · Высокий приоритет' },
  { value: 'dish-detail-error', label: 'Состав · Ошибка загрузки' },
  { value: 'dish-detail-slow', label: 'Состав · Загрузка' },
  { value: 'dish-detail-missing', label: 'Состав · Не найден' },
  { value: 'procurement-empty', label: 'Закупки · Нет рекомендаций' },
  { value: 'procurement-expired', label: 'Закупки · Остаток просрочен' },
  { value: 'stop-list-empty', label: 'Стоп-лист · Все блюда доступны' },
  { value: 'stop-list-expired', label: 'Стоп-лист · Остаток просрочен' },
  { value: 'stop-list-recipe', label: 'Стоп-лист · Рецептура изменена' },
  { value: 'order-detail-demo', label: 'Детали заказа · Три ингредиента' },
  { value: 'order-detail-split', label: 'Детали заказа · Несколько партий' },
  { value: 'order-not-found', label: 'Детали заказа · Не найден' },
  { value: 'dishes-empty', label: 'Блюда · Пустой каталог' },
  { value: 'dishes-error', label: 'Блюда · Ошибка каталога' },
  { value: 'dishes-slow', label: 'Блюда · Загрузка' },
  { value: 'order-check-slow', label: 'Заказ · Медленная проверка' },
  { value: 'order-check-error', label: 'Заказ · Ошибка проверки' },
  { value: 'order-check-shortage', label: 'Заказ · Дефицит при проверке' },
  { value: 'order-submit-slow', label: 'Заказ · Медленная отправка' },
  { value: 'order-network', label: 'Заказ · Обрыв сети' },
  { value: 'order-lost-response', label: 'Заказ · Ответ потерян после успеха' },
  { value: 'order-shortage', label: 'Заказ · Остатки изменились после проверки' },
  { value: 'order-recipe-changed', label: 'Заказ · Новая рецептура' },
  { value: 'order-inactive', label: 'Заказ · Блюдо отключено' },
  { value: 'order-final-recipe', label: 'Заказ · Рецептура изменилась при оформлении' },
  { value: 'order-final-inactive', label: 'Заказ · Блюдо отключено при оформлении' },
  { value: 'order-validation', label: 'Заказ · 422 при оформлении' },
  { value: 'order-rate-limit', label: 'Заказ · 429 при оформлении' },
  { value: 'orders-empty', label: 'Заказы · Пустая история обеих вкладок' },
  { value: 'products-empty', label: 'Продукты · Пустой каталог' },
  { value: 'products-error', label: 'Продукты · Ошибка каталога' },
  { value: 'products-slow', label: 'Продукты · Загрузка' },
  { value: 'reception-slow', label: 'Приёмка · Отправка' },
  { value: 'reception-network', label: 'Приёмка · Нет связи' },
  { value: 'reception-lost-response', label: 'Приёмка · Ответ потерян после успеха' },
  { value: 'reception-validation', label: 'Приёмка · Ошибка последней строки' },
  { value: 'reception-conflict', label: 'Приёмка · Номер занят' },
  { value: 'reception-in-progress', label: 'Приёмка · Запрос ещё выполняется' },
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
