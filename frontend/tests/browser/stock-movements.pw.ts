import { backgroundRefresh } from './refresh'
import { test, expect, type Page } from '@playwright/test'

async function login(page: Page, role = 'admin', target = '/stock-movements') {
  await page.goto(`/login?returnTo=${encodeURIComponent(target)}`)
  if (role !== 'manager') await page.getByLabel('Сценарий API').selectOption(`role-${role.replaceAll('_', '-')}`)
  await page.getByLabel(/^Email/).fill('employee@example.test'); await page.getByLabel(/^Пароль/).fill('local-test-only'); await page.getByRole('button', { name: 'Войти', exact: true }).click()
}
for (const width of [360, 390, 1024, 1440]) {
  test(`historical values, filters, URL and pagination at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 1000 }); await login(page)
    await expect(page.getByText('Найдено движений: 92. Страница 1.')).toBeVisible()
    await expect(page.getByText('-12,125 мл', { exact: true }).filter({ visible: true }).first()).toBeVisible()
    await expect(page.getByText('987,875 мл', { exact: true }).filter({ visible: true }).first()).toBeVisible()
    await expect(page.getByText('+1000 мл', { exact: true }).filter({ visible: true }).first()).toBeVisible()
    for (const [kind, count] of [['receipt', 17], ['order_consumption', 16], ['manual_writeoff', 42], ['expiration', 17]] as const) {
      await page.getByLabel('Тип движения', { exact: true }).selectOption(kind)
      await page.getByRole('button', { name: 'Найти', exact: true }).click()
      await expect(page.getByText(`Найдено движений: ${count}. Страница 1.`)).toBeVisible()
      expect(new URL(page.url()).searchParams.get('type')).toBe(kind)
    }
    await expect(page.getByText('Система', { exact: true }).filter({ visible: true }).first()).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: `test-results/stock-movements-${width}.png`, fullPage: false })
    await page.reload(); await expect(page.getByLabel('Тип движения')).toHaveValue('expiration')
    await page.getByRole('button', { name: 'Сбросить фильтры', exact: true }).click(); await expect(page.getByText('Найдено движений: 92. Страница 1.')).toBeVisible()
    await page.getByRole('button', { name: 'Страница 2', exact: true }).click(); await expect(page.getByText('Найдено движений: 92. Страница 2.')).toBeVisible()
    await page.goBack(); await expect(page.getByText('Найдено движений: 92. Страница 1.')).toBeVisible()
  })
}
test('lazy supply link, warehouse entry and batch prefilter', async ({ page }) => {
  await login(page); await expect(page.getByText('Найдено движений: 92. Страница 1.')).toBeVisible()
  await page.getByRole('button', { name: 'Найти поставку', exact: true }).first().click()
  await expect(page.getByRole('link', { name: 'Поставка №ТЕСТ-101', exact: true }).first()).toHaveAttribute('href', '/supplies/00000000-0000-4000-8000-000000000100')
  await page.getByRole('link', { name: 'Партии на складе', exact: true }).first().click()
  await page.getByRole('link', { name: 'История партии · строка 1', exact: true }).click()
  await expect(page.getByText('Найдено движений: 17. Страница 1.')).toBeVisible()
  await expect(page.getByLabel('ID партии', { exact: true })).toHaveValue('00000000-0000-4000-8000-000000000201')
})
test('order entry filters its movements', async ({ page }) => {
  await login(page); await expect(page.getByText('Найдено движений: 92. Страница 1.')).toBeVisible()
  await page.getByRole('link', { name: 'К заказу', exact: true }).first().click()
  await page.getByRole('link', { name: 'История движений заказа', exact: true }).click()
  await expect(page.getByText('Найдено движений: 16. Страница 1.')).toBeVisible()
  expect(new URL(page.url()).searchParams.get('order_id')).toBe('77777777-7777-4777-8777-777777777777')
})
for (const role of ['manager', 'storekeeper']) {
  test(`${role} reads history with permissioned links`, async ({ page }) => {
    await login(page, role); await expect(page.getByText('Найдено движений: 92. Страница 1.')).toBeVisible()
    expect(await page.getByRole('link', { name: 'К заказу', exact: true }).count() > 0).toBe(role === 'manager')
  })
}
test('order operator cannot mount either history route', async ({ page }) => {
  await login(page, 'order_operator', '/'); await expect(page.getByRole('heading', { name: 'Требуют внимания' })).toBeVisible()
  const requests: string[] = []; page.on('request', (request) => { if (new URL(request.url()).pathname === '/api/v1/stock-movements') requests.push(request.url()) })
  for (const path of ['/stock-movements', '/movements']) { await page.goto(path); await expect(page.getByRole('heading', { name: 'Недостаточно прав' })).toBeVisible() }
  expect(requests).toEqual([])
})
test('loading, stale data, empty, invalid response and revoked access', async ({ page }) => {
  await login(page); await expect(page.getByText('Найдено движений: 92. Страница 1.')).toBeVisible()
  await backgroundRefresh(page, 'movements-slow', '/api/v1/stock-movements', 'Загрузка движений', page.getByText('Найдено движений: 92. Страница 1.'))
  await page.getByLabel('Сценарий API').selectOption('movements-error'); await expect(page.getByText(/Данные могут быть устаревшими/)).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('movements-empty'); await expect(page.getByRole('heading', { name: 'Движений пока нет' })).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('movements-invalid'); await expect(page.getByRole('heading', { name: 'Не удалось загрузить движения' })).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('role-admin'); await expect(page.getByText('Найдено движений: 92. Страница 1.')).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('movements-forbidden'); await expect(page.getByRole('heading', { name: 'Недостаточно прав' })).toBeVisible(); await expect(page.getByText('Найдено движений: 92. Страница 1.')).not.toBeVisible()
})
