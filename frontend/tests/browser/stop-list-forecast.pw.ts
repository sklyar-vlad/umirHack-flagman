import { backgroundRefresh } from './refresh'
import { test, expect, type Page } from '@playwright/test'

async function login(page: Page, role = 'admin', target = '/analytics/stop-list-forecast') {
  await page.goto(`/login?returnTo=${encodeURIComponent(target)}`)
  if (role !== 'manager') await page.getByLabel('Сценарий API').selectOption(`role-${role.replaceAll('_', '-')}`)
  await page.getByLabel(/^Email/).fill('employee@example.test')
  await page.getByLabel(/^Пароль/).fill('local-test-only')
  await page.getByRole('button', { name: 'Войти', exact: true }).click()
}
const ready = (page: Page, total = 5, number = 1, days = 7, hours = 72) => expect(page.getByText(`Активных блюд: ${total}. Страница ${number}. История: ${days} дн. Горизонт: ${hours} ч.`)).toBeVisible()
const cards = (page: Page) => page.getByRole('list', { name: 'Оценки остановки блюд' }).locator(':scope > li')

for (const width of [360, 390, 1024, 1440]) {
  test(`forecast states, settings, keyboard and pagination at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 1000 })
    const errors: string[] = [], queries: URL[] = []
    page.on('pageerror', (error) => errors.push(error.message))
    page.on('request', (request) => { if (new URL(request.url()).pathname === '/api/v1/analytics/stop-list-forecast') queries.push(new URL(request.url())) })
    await login(page); await ready(page)
    if (width === 1440) await expect(page.getByRole('link', { name: 'Прогноз', exact: true })).toHaveAttribute('aria-current', 'page')
    await expect(cards(page)).toHaveCount(5)
    await expect(cards(page).nth(0)).toContainText('Уже недоступно')
    await expect(cards(page).nth(1)).toContainText('Истечение срока партии')
    await expect(cards(page).nth(1)).toContainText('13.10.2026, 12:30')
    await expect(cards(page).nth(1)).toContainText('Недостаточно данных')
    await expect(cards(page).nth(2)).toContainText('Ожидаемый расход')
    await expect(cards(page).nth(3)).toContainText('Недостаточно истории; по срокам остановки в горизонте не найдено')
    await expect(cards(page).nth(4)).toContainText('Остановка в пределах горизонта не ожидается')
    await expect(cards(page).nth(4)).toContainText('Отсутствие даты не означает, что блюдо никогда не попадёт в стоп-лист.')
    const assumptions = cards(page).nth(1).locator('summary')
    await assumptions.focus(); await page.keyboard.press('Enter')
    await expect(cards(page).nth(1).getByText(/Нет истории расхода в текущей единице продукта/)).toBeVisible()
    await expect(cards(page).nth(1).getByText('Будущие поставки не учитываются', { exact: true })).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: `test-results/forecast-${width}.png`, fullPage: false })
    await page.getByLabel('Сценарий API').selectOption('forecast-demo'); await ready(page, 27)
    await page.getByLabel('Блюд на странице').selectOption('20'); await ready(page, 27)
    await page.getByRole('button', { name: 'Страница 2', exact: true }).click(); await ready(page, 27, 2)
    await page.getByLabel('История расхода, дней').fill('90')
    await page.getByLabel('Горизонт прогноза, часов').fill('720')
    await page.getByRole('button', { name: 'Применить', exact: true }).click(); await ready(page, 27, 1, 90, 720)
    expect(new URL(page.url()).searchParams.has('page')).toBe(false)
    await page.reload(); await ready(page, 5, 1, 90, 720)
    await expect(page.getByLabel('История расхода, дней')).toHaveValue('90')
    await expect(page.getByLabel('Горизонт прогноза, часов')).toHaveValue('720')
    await page.getByLabel('Сценарий API').selectOption('forecast-demo'); await ready(page, 27, 1, 90, 720)
    await page.goBack(); await ready(page, 27, 2)
    await page.getByRole('button', { name: 'Сбросить настройки', exact: true }).click(); await ready(page, 27)
    expect(new URL(page.url()).search).toBe('')
    const recipe = cards(page).first().getByRole('link', { name: 'Рецептура блюда', exact: true })
    await recipe.focus(); await page.keyboard.press('Enter')
    await expect(page.getByRole('heading', { name: 'Редактирование блюда', exact: true })).toBeVisible()
    await page.goBack(); await ready(page, 27)
    expect(queries.every((url) => [...url.searchParams.keys()].every((key) => ['page', 'page_size', 'lookback_days', 'horizon_hours'].includes(key)))).toBe(true)
    expect(errors).toEqual([])
  })
}

test('manager links, legacy forecast route and analytics navigation', async ({ page }) => {
  await login(page, 'manager', '/forecast?lookback_days=1&horizon_hours=1&token=discard'); await ready(page, 5, 1, 1, 1)
  expect(new URL(page.url()).pathname).toBe('/analytics/stop-list-forecast')
  expect(new URL(page.url()).searchParams.has('token')).toBe(false)
  await expect(page.getByRole('link', { name: 'Карточка продукта', exact: true })).toHaveCount(0)
  await expect(cards(page).first().getByRole('link', { name: 'Рецептура блюда' })).toHaveAttribute('href', '/dishes/00000000-0000-4000-8000-000000000800')
  const nav = page.getByRole('navigation', { name: 'Отчёты аналитики' })
  await nav.getByRole('link', { name: 'Потери от списаний', exact: true }).click(); await expect(page.getByRole('heading', { name: 'Потери от списаний', exact: true })).toBeVisible()
  await nav.getByRole('link', { name: 'Недостача ингредиентов', exact: true }).click(); await expect(page.getByRole('heading', { name: 'Недостача ингредиентов', exact: true })).toBeVisible()
  await nav.getByRole('link', { name: 'Прогноз стоп-листа', exact: true }).click(); await ready(page)
  await page.goto('/forecast'); await ready(page)
})

for (const role of ['storekeeper', 'order_operator']) {
  test(`${role} cannot mount forecast requests`, async ({ page }) => {
    await login(page, role, '/'); await expect(page.getByRole('heading', { name: 'Требуют внимания' })).toBeVisible()
    const queries: string[] = []
    page.on('request', (request) => { if (new URL(request.url()).pathname === '/api/v1/analytics/stop-list-forecast') queries.push(request.url()) })
    await page.goto('/analytics/stop-list-forecast'); await expect(page.getByRole('heading', { name: 'Недостаточно прав' })).toBeVisible()
    await page.goto('/forecast'); await expect(page.getByRole('heading', { name: 'Недостаточно прав' })).toBeVisible()
    expect(queries).toEqual([])
  })
}

test('loading, stale error, retry, empty, invalid response and access denial', async ({ page }) => {
  await login(page); await ready(page)
  await backgroundRefresh(page, 'forecast-slow', '/api/v1/analytics/stop-list-forecast', 'Загрузка прогноза', cards(page).first())
  await page.getByLabel('Сценарий API').selectOption('forecast-error'); await expect(page.getByText(/Данные могут быть устаревшими/)).toBeVisible(); await ready(page)
  await page.getByLabel('Сценарий API').selectOption('success'); await expect(page.getByRole('heading', { name: 'Не удалось загрузить прогноз' })).not.toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('forecast-empty'); await expect(page.getByRole('heading', { name: 'Активных блюд пока нет' })).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('forecast-invalid'); await expect(page.getByRole('heading', { name: 'Не удалось загрузить прогноз' })).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('role-admin'); await ready(page)
  await page.getByLabel('Сценарий API').selectOption('forecast-forbidden'); await expect(page.getByRole('heading', { name: 'Недостаточно прав' })).toBeVisible()
  await expect(cards(page)).toHaveCount(0)
})

test('form ranges, URL duplicates, empty page and cache isolation', async ({ page }) => {
  await login(page, 'admin', '/analytics/stop-list-forecast?page=99'); await expect(page.getByRole('heading', { name: 'На этой странице нет блюд' })).toBeVisible()
  await page.getByRole('button', { name: 'К первой странице', exact: true }).click(); await ready(page)
  await page.getByLabel('История расхода, дней').fill('0'); await page.getByLabel('Горизонт прогноза, часов').fill('721')
  await page.getByRole('button', { name: 'Применить', exact: true }).click()
  await expect(page.getByLabel('История расхода, дней')).toHaveAttribute('aria-invalid', 'true')
  await expect(page.getByLabel('Горизонт прогноза, часов')).toHaveAttribute('aria-invalid', 'true')
  await page.getByLabel('История расхода, дней').fill('1'); await page.getByLabel('Горизонт прогноза, часов').fill('1')
  await page.getByLabel('Сценарий API').selectOption('forecast-slow'); await expect(page.getByRole('status', { name: 'Загрузка прогноза' })).not.toBeVisible()
  await page.getByRole('button', { name: 'Применить', exact: true }).click(); await expect(page.getByRole('status', { name: 'Загрузка прогноза' })).toBeVisible()
  await expect(cards(page)).toHaveCount(0); await ready(page, 5, 1, 1, 1)
  await page.goto('/analytics/stop-list-forecast?lookback_days=1&lookback_days=2'); await expect(page.getByRole('heading', { name: 'Некорректные настройки' })).toBeVisible()
  await expect(cards(page)).toHaveCount(0)
})
