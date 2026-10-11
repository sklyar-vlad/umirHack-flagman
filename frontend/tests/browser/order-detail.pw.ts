import { expect, test, type Page } from '@playwright/test'

const id = '77777777-7777-4777-8777-777777777777'
const history = '/orders?orders_search=247&orders_page_size=20&attempts_page=2&attempts_reason_code=RECIPE_CHANGED'
const target = `/orders/${id}?returnTo=${encodeURIComponent(history)}`
async function login(page: Page, path = target, role = 'manager') {
  await page.goto(`/login?returnTo=${encodeURIComponent(path)}`)
  if (role !== 'manager') await page.getByLabel('Сценарий API').selectOption(`role-${role.replaceAll('_', '-')}`)
  await page.getByLabel(/^Email/).fill('employee@example.test')
  await page.getByLabel(/^Пароль/).fill('local-test-only')
  await page.getByRole('button', { name: 'Войти', exact: true }).click()
}
const snapshot = (page: Page) => page.getByRole('region', { name: 'Позиция: Пицца «Маргарита»' })
const warehouseLink = (page: Page) => page.getByRole('link', { name: 'Партия на складе: Моцарелла, поставка 104, строка 1' }).filter({ visible: true })

for (const width of [360, 390, 1024, 1440]) {
  test(`immutable detail, exact split batches, keyboard warehouse and filtered return at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 1000 })
    const errors: string[] = [], dishes: string[] = []
    page.on('pageerror', (error) => errors.push(error.message))
    page.on('request', (request) => { if (new URL(request.url()).pathname.startsWith('/api/v1/dishes')) dishes.push(request.url()) })
    await login(page)
    await expect(page.getByRole('heading', { name: 'Заказ №247' })).toBeVisible()
    await page.getByLabel('Сценарий API').selectOption('order-detail-demo')
    await expect(snapshot(page).getByText('240 г', { exact: true }).filter({ visible: true })).toBeVisible()
    await expect(snapshot(page).getByText('Томаты', { exact: true }).filter({ visible: true })).toBeVisible()
    await expect(snapshot(page).getByText('Тесто для пиццы', { exact: true }).filter({ visible: true })).toBeVisible()
    await expect(page.getByText('09.10.2026, 12:00:00', { exact: true })).toBeVisible()
    await expect(page.getByText('Рецептура: 1', { exact: true })).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: `test-results/order-detail-${width}.png`, fullPage: true })
    await page.getByLabel('Сценарий API').selectOption('order-detail-split')
    const allocations = width <= 1100 ? snapshot(page).getByRole('listitem') : snapshot(page).getByRole('row').filter({ has: page.locator('td') })
    await expect(allocations).toHaveCount(4)
    await expect(allocations.nth(0)).toContainText('120,001 г'); await expect(allocations.nth(0)).toContainText('0 г')
    await expect(allocations.nth(1)).toContainText('119,999 г'); await expect(allocations.nth(1)).toContainText('840,001 г')
    await expect(allocations.nth(1)).toContainText('Поставка 105, строка 4')
    await page.screenshot({ path: `test-results/order-detail-split-${width}.png`, fullPage: true })
    await warehouseLink(page).focus(); await page.keyboard.press('Enter')
    const selected = page.getByRole('region', { name: 'Выбор из поставки' })
    await expect(selected.getByLabel('Показать партии')).toHaveValue('all')
    await expect(selected.getByText('Израсходована', { exact: true })).toBeVisible()
    await expect(selected.getByText('Выбранная поставка').first()).toBeVisible()
    expect(new URL(page.url()).searchParams.get('product_id')).toBe('11111111-1111-4111-8111-111111111111')
    expect(new URL(page.url()).searchParams.get('supply_id')).toBe('44444444-4444-4444-8444-444444444444')
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.goBack(); await expect(page.getByRole('heading', { name: 'Заказ №247' })).toBeVisible()
    await page.getByRole('link', { name: 'К истории заказов' }).focus(); await page.keyboard.press('Enter')
    await expect(page.getByRole('link', { name: 'Заказ 247', exact: true })).toBeVisible()
    expect(new URL(page.url()).searchParams.get('attempts_page')).toBe('2')
    expect(new URL(page.url()).searchParams.get('attempts_reason_code')).toBe('RECIPE_CHANGED')
    await page.getByRole('link', { name: 'Заказ 247', exact: true }).click()
    await page.reload(); await expect(page.getByRole('heading', { name: 'Заказ №247' })).toBeVisible()
    expect(dishes).toEqual([]); expect(errors).toEqual([])
  })
}

test('loading, keyboard retry, stale snapshot, cached resource 404 and local 403', async ({ page }) => {
  await login(page); await expect(page.getByRole('heading', { name: 'Заказ №247' })).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('slow')
  await expect(page.getByRole('status', { name: 'Загрузка заказа' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Обновить заказ' })).toBeDisabled()
  await expect(page.getByRole('status', { name: 'Загрузка заказа' })).toHaveCount(0)
  await page.getByLabel('Сценарий API').selectOption('network-error')
  await expect(page.getByText(/Данные могут быть устаревшими/)).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Заказ №247' })).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('success')
  await page.getByRole('button', { name: 'Обновить заказ' }).focus(); await page.keyboard.press('Enter')
  await expect(page.getByRole('heading', { name: 'Не удалось загрузить заказ' })).toHaveCount(0)
  await page.getByLabel('Сценарий API').selectOption('order-not-found')
  await expect(page.getByRole('heading', { name: 'Заказ не найден' })).toBeVisible()
  await expect(snapshot(page)).toHaveCount(0)
  await page.getByLabel('Сценарий API').selectOption('success')
  await expect(page.getByRole('heading', { name: 'Заказ №247' })).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('forbidden')
  await expect(page.getByRole('heading', { name: 'Недостаточно прав' })).toBeVisible()
  await expect(snapshot(page)).toHaveCount(0)
  await expect(page.getByRole('banner').getByText('Тестовый сотрудник')).toBeVisible()
})

for (const role of ['admin', 'order_operator']) {
  test(`${role} related navigation follows warehouse/supply permissions`, async ({ page }) => {
    const supplies: string[] = []
    page.on('request', (request) => { if (new URL(request.url()).pathname.startsWith('/api/v1/supplies')) supplies.push(request.url()) })
    await login(page, target, role); await expect(page.getByRole('heading', { name: 'Заказ №247' })).toBeVisible()
    await page.getByLabel('Сценарий API').selectOption('order-detail-demo')
    const href = await warehouseLink(page).getAttribute('href')
    expect(new URL(href!, 'http://internal.test').searchParams.has('supply_id')).toBe(role === 'admin')
    await warehouseLink(page).focus(); await page.keyboard.press('Enter')
    await expect(page.getByRole('heading', { name: 'Склад', exact: true })).toBeVisible()
    if (role === 'order_operator') {
      await expect(page.getByRole('region', { name: 'Выбор из поставки' })).toHaveCount(0)
      await expect(page.getByRole('heading', { name: 'Моцарелла', exact: true })).toBeVisible()
      expect(supplies).toEqual([])
    } else await expect(page.getByRole('region', { name: 'Выбор из поставки' }).getByLabel('Показать партии')).toHaveValue('all')
  })
}

test('first-load missing resource and invalid UUID retain a safe history exit', async ({ page }) => {
  await login(page, '/orders/99999999-9999-4999-8999-999999999999?returnTo=https://evil.test')
  await expect(page.getByRole('heading', { name: 'Заказ не найден' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'К истории заказов' })).toHaveAttribute('href', '/orders')
  await page.goto('/orders/wrong')
  await expect(page.getByRole('heading', { name: 'Некорректный адрес заказа' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Попробовать снова' })).toHaveCount(0)
})

test('F10 confirmed order opens the same immutable allocation snapshot through GET', async ({ page }) => {
  await login(page, '/orders/new')
  await page.getByRole('button', { name: 'Добавить Суп дня', exact: true }).click()
  await page.getByLabel(/^Порции: Суп дня/).fill('6')
  const submit = page.getByRole('button', { name: 'Оформить заказ', exact: true })
  await expect(submit).toBeEnabled(); await submit.click()
  await expect(page.getByRole('heading', { name: 'Заказ №1001' })).toBeVisible()
  const reads: string[] = [], dishes: string[] = []
  page.on('request', (request) => {
    const path = new URL(request.url()).pathname
    if (path.startsWith('/api/v1/orders/')) reads.push(path)
    if (path.startsWith('/api/v1/dishes')) dishes.push(path)
  })
  const link = page.getByRole('link', { name: 'К заказу №1001' })
  const href = await link.getAttribute('href')
  await link.focus(); await page.keyboard.press('Enter')
  await expect(page.getByRole('heading', { name: 'Детали заказа' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Заказ №1001' })).toBeVisible()
  const result = page.getByRole('region', { name: 'Позиция: Суп дня' })
  await expect(result).toContainText('500 мл'); await expect(result).toContainText('50 мл')
  expect(reads).toEqual([`/api/v1${href}`]); expect(dishes).toEqual([])
})
