import { test, expect, type Page } from '@playwright/test'
async function login(page: Page, role = 'admin', target = '/analytics/write-offs') {
  await page.goto(`/login?returnTo=${encodeURIComponent(target)}`)
  if (role !== 'manager') await page.getByLabel('Сценарий API').selectOption(`role-${role.replaceAll('_', '-')}`)
  await page.getByLabel(/^Email/).fill('employee@example.test'); await page.getByLabel(/^Пароль/).fill('local-test-only'); await page.getByRole('button', { name: 'Войти', exact: true }).click()
}
for (const width of [360, 390, 1024, 1440]) {
  test(`loss quantities, filters and history at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 1000 }); await login(page)
    await expect(page.getByText('Групп продуктов и единиц: 27. Страница 1.')).toBeVisible()
    await expect(page.getByText('199999999999,998 г', { exact: true }).filter({ visible: true }).first()).toBeVisible()
    await expect(page.getByText('3200 мл', { exact: true }).filter({ visible: true }).first()).toBeVisible()
    await expect(page.getByText('2 шт.', { exact: true }).filter({ visible: true }).first()).toBeVisible()
    await page.getByLabel('Причина списания').selectOption('expiration'); await page.getByRole('button', { name: 'Найти', exact: true }).click()
    await expect(page.getByText('Групп продуктов и единиц: 2. Страница 1.')).toBeVisible()
    expect(new URL(page.url()).searchParams.get('reason')).toBe('expiration')
    await page.getByLabel('ID продукта', { exact: true }).fill('00000000-0000-4000-8000-000000000010')
    await page.getByLabel('Потери с (включительно)').fill('2026-10-10T11:00')
    await page.getByLabel('Потери до (не включая)').fill('2026-10-10T12:00')
    await page.getByRole('button', { name: 'Найти', exact: true }).click()
    await expect(page.getByText('Групп продуктов и единиц: 1. Страница 1.')).toBeVisible()
    await page.screenshot({ path: `test-results/losses-${width}.png`, fullPage: false })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.reload(); await expect(page.getByLabel('Причина списания')).toHaveValue('expiration')
    const link = page.getByRole('link', { name: 'Движения просрочки', exact: true }).first()
    await link.focus(); await page.keyboard.press('Enter')
    await expect(page.getByText('Найдено движений: 15. Страница 1.')).toBeVisible()
    const params = new URL(page.url()).searchParams
    expect(params.get('type')).toBe('expiration'); expect(params.get('from')).toBe('2026-10-10T08:00:00Z'); expect(params.get('to')).toBe('2026-10-10T09:00:00Z')
    await page.goBack(); await expect(page.getByText('Групп продуктов и единиц: 1. Страница 1.')).toBeVisible()
    await page.getByRole('button', { name: 'Сбросить фильтры', exact: true }).click(); await expect(page.getByText('Групп продуктов и единиц: 27. Страница 1.')).toBeVisible(); await page.getByLabel('Записей на странице').selectOption('20')
    await expect(page.getByText('Групп продуктов и единиц: 27. Страница 1.')).toBeVisible()
    await page.getByRole('button', { name: 'Страница 2', exact: true }).click(); await expect(page.getByText('Групп продуктов и единиц: 27. Страница 2.')).toBeVisible()
  })
}
for (const role of ['storekeeper', 'order_operator']) {
  test(`${role} cannot query either report route`, async ({ page }) => {
    await login(page, role, '/'); await expect(page.getByRole('heading', { name: 'Требуют внимания' })).toBeVisible()
    const requests: string[] = []; page.on('request', (request) => { if (new URL(request.url()).pathname === '/api/v1/analytics/write-offs') requests.push(request.url()) })
    for (const target of ['/analytics', '/analytics/write-offs']) { await page.goto(target); await expect(page.getByRole('heading', { name: 'Недостаточно прав' })).toBeVisible() }
    expect(requests).toEqual([])
  })
}
test('manager can read report and manual-only movement links', async ({ page }) => {
  await login(page, 'manager'); await expect(page.getByText('Групп продуктов и единиц: 27. Страница 1.')).toBeVisible()
  await page.getByLabel('Причина списания').selectOption('manual_writeoff'); await page.getByRole('button', { name: 'Найти', exact: true }).click()
  await expect(page.getByText('Групп продуктов и единиц: 27. Страница 1.')).toBeVisible()
  await expect(page.getByRole('link', { name: 'Движения просрочки', exact: true })).toHaveCount(0)
  await expect(page.getByRole('link', { name: 'Ручные движения', exact: true }).first()).toHaveAttribute('href', /type=manual_writeoff/)
})
test('order consumption leaves losses unchanged; loading, errors, empty and revoked access', async ({ page }) => {
  await login(page); await expect(page.getByText('Групп продуктов и единиц: 27. Страница 1.')).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('losses-order'); await expect(page.getByText('3200 мл', { exact: true }).filter({ visible: true }).first()).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('losses-slow'); await expect(page.getByRole('status', { name: 'Загрузка потерь' })).toBeVisible(); await expect(page.getByRole('status', { name: 'Загрузка потерь' })).not.toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('losses-error'); await expect(page.getByText(/Данные могут быть устаревшими/)).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('losses-empty'); await expect(page.getByRole('heading', { name: 'Потерь пока нет' })).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('losses-invalid'); await expect(page.getByRole('heading', { name: 'Не удалось загрузить потери' })).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('role-admin'); await expect(page.getByText('Групп продуктов и единиц: 27. Страница 1.')).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('losses-forbidden'); await expect(page.getByRole('heading', { name: 'Недостаточно прав' })).toBeVisible(); await expect(page.getByText('Групп продуктов и единиц: 27. Страница 1.')).not.toBeVisible()
})
