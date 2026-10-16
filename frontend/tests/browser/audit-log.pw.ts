import { backgroundRefresh } from './refresh'
import { test, expect, type Page } from '@playwright/test'

async function login(page: Page, role = 'admin', target = '/admin/audit-log') {
  await page.goto(`/login?returnTo=${encodeURIComponent(target)}`)
  if (role !== 'manager') await page.getByLabel('Сценарий API').selectOption(`role-${role.replaceAll('_', '-')}`)
  await page.getByLabel(/^Email/).fill('employee@example.test'); await page.getByLabel(/^Пароль/).fill('local-test-only'); await page.getByRole('button', { name: 'Войти', exact: true }).click()
}
for (const width of [360, 390, 1024, 1440]) {
  test(`audit filters, details and keyboard at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 1000 }); await login(page)
    await expect(page.getByText('Найдено записей: 65. Страница 1.')).toBeVisible()
    await page.getByText('Фильтры по автору, объекту и периоду', { exact: true }).click(); await page.getByLabel('Автор (UUID или system)').fill('system'); await page.getByLabel('Действие', { exact: true }).selectOption('expiration'); await page.getByRole('button', { name: 'Найти', exact: true }).click()
    await expect(page.getByText('Найдено записей: 1. Страница 1.')).toBeVisible()
    expect(new URL(page.url()).searchParams.get('actor_id')).toBe('system')
    const button = page.getByRole('button', { name: 'Изменения записи 00000000-0000-4000-8000-000000000901' })
    await button.focus(); await page.keyboard.press('Enter')
    const dialog = page.getByRole('dialog', { name: 'Изменения записи' })
    await expect(dialog).toContainText('Система'); await expect(dialog).toContainText('1000.000'); await expect(dialog).toContainText('0.000')
    await page.screenshot({ path: `test-results/audit-details-${width}.png`, fullPage: false })
    await page.keyboard.press('Escape'); await expect(dialog).not.toBeVisible(); await expect(button).toBeFocused()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: `test-results/audit-${width}.png`, fullPage: true })
    await page.reload(); await expect(page.getByLabel('Автор (UUID или system)')).toHaveValue('system')
    await page.getByRole('button', { name: 'Сбросить фильтры', exact: true }).click(); await expect(page.getByText('Найдено записей: 65. Страница 1.')).toBeVisible()
    await page.getByRole('button', { name: 'Страница 2', exact: true }).click(); await expect(page.getByText('Найдено записей: 65. Страница 2.')).toBeVisible()
  })
}
for (const role of ['manager', 'storekeeper', 'order_operator']) {
  test(`${role} cannot read either journal URL`, async ({ page }) => {
    await login(page, role, '/'); await expect(page.getByRole('heading', { name: 'Требуют внимания' })).toBeVisible()
    const requests: string[] = []; page.on('request', (request) => { if (new URL(request.url()).pathname === '/api/v1/audit-log') requests.push(request.url()) })
    for (const path of ['/admin/audit-log', '/admin/audit']) { await page.goto(path); await expect(page.getByRole('heading', { name: 'Недостаточно прав' })).toBeVisible() }
    expect(requests).toEqual([])
  })
}
test('loading, stale retry, empty, invalid response and revoked access', async ({ page }) => {
  await login(page); await expect(page.getByText('Найдено записей: 65. Страница 1.')).toBeVisible()
  await backgroundRefresh(page, 'audit-slow', '/api/v1/audit-log', 'Загрузка журнала', page.getByText('Найдено записей: 65. Страница 1.'))
  await page.getByLabel('Сценарий API').selectOption('audit-error'); await expect(page.getByText(/Данные могут быть устаревшими/)).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('audit-empty'); await expect(page.getByRole('heading', { name: 'Журнал пока пуст' })).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('audit-invalid'); await expect(page.getByRole('heading', { name: 'Не удалось загрузить журнал' })).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('role-admin'); await expect(page.getByText('Найдено записей: 65. Страница 1.')).toBeVisible()
  await page.getByRole('button', { name: 'Изменения записи 00000000-0000-4000-8000-000000000900' }).click(); await expect(page.getByRole('dialog')).toBeVisible(); await page.keyboard.press('Escape')
  await page.getByLabel('Сценарий API').selectOption('audit-forbidden'); await expect(page.getByRole('heading', { name: 'Недостаточно прав' })).toBeVisible(); await expect(page.getByText('Найдено записей: 65. Страница 1.')).not.toBeVisible()
})

test('historical record survives a link to an unavailable current product', async ({ page }) => {
  await login(page); await expect(page.getByText('Найдено записей: 65. Страница 1.')).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('product-not-found')
  await page.getByRole('button', { name: 'Изменения записи 00000000-0000-4000-8000-000000000900' }).click()
  await page.getByRole('link', { name: 'Открыть текущий объект' }).click(); await expect(page.getByRole('heading', { name: 'Продукт не найден' })).toBeVisible()
  await page.goBack(); await expect(page.getByText('Найдено записей: 65. Страница 1.')).toBeVisible()
})
