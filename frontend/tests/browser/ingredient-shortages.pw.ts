import { test, expect, type Page } from '@playwright/test'

async function login(page: Page, role = 'admin', target = '/analytics/ingredient-shortages') {
  await page.goto(`/login?returnTo=${encodeURIComponent(target)}`)
  if (role !== 'manager') await page.getByLabel('Сценарий API').selectOption(`role-${role.replaceAll('_', '-')}`)
  await page.getByLabel(/^Email/).fill('employee@example.test')
  await page.getByLabel(/^Пароль/).fill('local-test-only')
  await page.getByRole('button', { name: 'Войти', exact: true }).click()
}
const ready = (page: Page, total = 27, number = 1) => expect(page.getByText(`Групп продуктов и единиц: ${total}. Страница ${number}.`)).toBeVisible()
const records = (page: Page) => page.getByRole('table', { name: 'Недостача по продуктам' }).or(page.getByRole('region', { name: 'Недостача по продуктам' })).filter({ visible: true })

for (const width of [360, 390, 1024, 1440]) {
  test(`exact historical shortage, period, pagination and keyboard links at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 1000 })
    const errors: string[] = [], queries: URL[] = []
    page.on('pageerror', (error) => errors.push(error.message))
    page.on('request', (request) => { if (new URL(request.url()).pathname === '/api/v1/analytics/ingredient-shortages') queries.push(new URL(request.url())) })
    await login(page); await ready(page, 1)
    await page.getByLabel('Сценарий API').selectOption('shortages-demo'); await ready(page)
    await expect(records(page)).toContainText('200000027600,021 г')
    await expect(records(page)).toContainText('3200,001 мл')
    await expect(records(page)).toContainText('2 шт.')
    await expect(page.getByText('Неудовлетворённый спрос из отклонённых заказов. Это не подтверждённые будущие заказы.')).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: `test-results/shortages-${width}.png`, fullPage: false })
    await page.getByLabel('Записей на странице').selectOption('20'); await ready(page)
    await page.getByRole('button', { name: 'Страница 2', exact: true }).click(); await ready(page, 27, 2)
    await page.getByLabel('Отказы с (включительно)').fill('2026-10-10T12:00')
    await page.getByLabel('Отказы до (не включая)').fill('2026-10-10T12:01')
    await page.getByRole('button', { name: 'Найти', exact: true }).click(); await ready(page, 27)
    expect(new URL(page.url()).searchParams.has('page')).toBe(false)
    await expect(records(page)).toContainText('99999999999,999 г')
    await page.reload(); await ready(page, 0)
    await page.getByLabel('Сценарий API').selectOption('shortages-demo'); await ready(page, 27)
    await expect(page.getByLabel('Отказы с (включительно)')).toHaveValue('2026-10-10T12:00')
    const history = page.getByRole('link', { name: 'История отказов за период', exact: true }).filter({ visible: true }).first()
    await history.focus(); await page.keyboard.press('Enter')
    await expect(page.getByText('Найдено отказов: 27. Страница 1.')).toBeVisible()
    const params = new URL(page.url()).searchParams
    expect(params.get('attempts_from')).toBe('2026-10-10T09:00:00Z')
    expect(params.get('attempts_to')).toBe('2026-10-10T09:01:00Z')
    expect(params.get('attempts_reason_code')).toBe('INSUFFICIENT_STOCK')
    expect(params.has('product_id')).toBe(false)
    await page.goBack(); await ready(page, 27)
    await page.getByRole('button', { name: 'Сбросить фильтры', exact: true }).click(); await ready(page)
    expect(new URL(page.url()).search).toBe('')
    expect(queries.every((url) => [...url.searchParams.keys()].every((key) => ['page', 'page_size', 'from', 'to'].includes(key)))).toBe(true)
    expect(errors).toEqual([])
  })
}

test('manager links and analytics navigation preserve the loss report', async ({ page }) => {
  await login(page, 'manager'); await ready(page, 1)
  await expect(page.getByRole('link', { name: 'Карточка продукта', exact: true })).toHaveCount(0)
  await expect(page.getByRole('link', { name: 'Продукт на складе', exact: true }).filter({ visible: true }).first()).toHaveAttribute('href', /product_id=00000000-0000-4000-8000-000000000700/)
  await page.getByRole('navigation', { name: 'Отчёты аналитики' }).getByRole('link', { name: 'Потери от списаний', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Потери от списаний', exact: true })).toBeVisible()
  await ready(page, 27)
  await page.getByRole('navigation', { name: 'Отчёты аналитики' }).getByRole('link', { name: 'Недостача ингредиентов', exact: true }).click(); await ready(page, 1)
})

for (const role of ['storekeeper', 'order_operator']) {
  test(`${role} cannot mount analytics queries`, async ({ page }) => {
    await login(page, role, '/')
    await expect(page.getByRole('heading', { name: 'Требуют внимания' })).toBeVisible()
    const queries: string[] = []
    page.on('request', (request) => { if (new URL(request.url()).pathname === '/api/v1/analytics/ingredient-shortages') queries.push(request.url()) })
    await page.goto('/analytics/ingredient-shortages'); await expect(page.getByRole('heading', { name: 'Недостаточно прав' })).toBeVisible()
    expect(queries).toEqual([])
  })
}

test('loading, stale error, retry, empty, invalid response and revoked access', async ({ page }) => {
  await login(page); await ready(page, 1)
  await page.getByLabel('Сценарий API').selectOption('shortages-slow'); await expect(page.getByRole('status', { name: 'Загрузка недостачи' })).toBeVisible(); await expect(page.getByRole('status', { name: 'Загрузка недостачи' })).not.toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('shortages-error'); await expect(page.getByText(/Данные могут быть устаревшими/)).toBeVisible(); await ready(page, 1)
  await page.getByLabel('Сценарий API').selectOption('success'); await expect(page.getByRole('heading', { name: 'Не удалось загрузить недостачу' })).not.toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('shortages-empty'); await expect(page.getByRole('heading', { name: 'Отказов из-за недостачи пока нет' })).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('shortages-invalid'); await expect(page.getByRole('heading', { name: 'Не удалось загрузить недостачу' })).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('role-admin'); await ready(page, 1)
  await page.getByLabel('Сценарий API').selectOption('shortages-forbidden'); await expect(page.getByRole('heading', { name: 'Недостаточно прав' })).toBeVisible()
  await expect(records(page)).toHaveCount(0)
})

test('invalid URL, reversed form period, empty period/page and login allowlist', async ({ page }) => {
  await login(page, 'admin', '/analytics/ingredient-shortages?page=99&token=discard&product_id=discard')
  await expect(page.getByRole('heading', { name: 'На этой странице нет недостачи' })).toBeVisible()
  expect(new URL(page.url()).searchParams.has('token')).toBe(false)
  expect(new URL(page.url()).searchParams.has('product_id')).toBe(false)
  await page.getByRole('button', { name: 'К первой странице', exact: true }).click(); await ready(page, 1)
  await page.getByLabel('Отказы с (включительно)').fill('2026-10-11T12:00')
  await page.getByLabel('Отказы до (не включая)').fill('2026-10-10T12:00')
  await page.getByRole('button', { name: 'Найти', exact: true }).click()
  await expect(page.getByLabel('Отказы до (не включая)')).toHaveAttribute('aria-invalid', 'true')
  await page.getByLabel('Отказы до (не включая)').fill('2026-10-12T12:00')
  await page.getByRole('button', { name: 'Найти', exact: true }).click(); await expect(page.getByRole('heading', { name: 'Ничего не найдено' })).toBeVisible()
  await page.goto('/analytics/ingredient-shortages?from=bad'); await expect(page.getByRole('heading', { name: 'Некорректные фильтры' })).toBeVisible()
})

test('explicit MSW: three checks, one saved refusal and identical replay yield one new attempt', async ({ page }) => {
  const posts: { body: string; key: string }[] = []
  page.on('request', (request) => { if (new URL(request.url()).pathname === '/api/v1/orders' && request.method() === 'POST') posts.push({ body: request.postData()!, key: request.headers()['idempotency-key'] }) })
  await login(page, 'manager', '/orders/new')
  await page.getByRole('button', { name: 'Добавить Суп дня', exact: true }).click()
  const create = page.getByRole('button', { name: 'Оформить заказ', exact: true })
  await expect(create).toBeEnabled()
  for (let i = 0; i < 3; i++) { await page.getByRole('button', { name: 'Проверить ещё раз', exact: true }).click(); await expect(create).toBeEnabled() }
  const read = () => page.evaluate(async () => (await (await fetch('/api/v1/analytics/ingredient-shortages')).json()).items)
  const before = await read(); expect(before).toHaveLength(1)
  await page.getByLabel('Сценарий API').selectOption('order-shortage'); await create.click()
  await expect(page.getByText(/Сохранённая попытка:/)).toBeVisible()
  const after = await read(); expect(after).toHaveLength(2)
  expect(after.find((row: { product_id: string }) => row.product_id.endsWith('000000000010')).rejected_attempts_count).toBe(1)
  expect(posts).toHaveLength(1)
  const replay = await page.evaluate(async ({ body, key }) => {
    const response = await fetch('/api/v1/orders', { method: 'POST', headers: { 'Content-Type': 'application/json', 'Idempotency-Key': key, 'X-CSRF-Token': 'mock-csrf-for-local-tests' }, body })
    return { status: response.status, body: await response.json() }
  }, posts[0])
  expect(replay.status).toBe(409); expect(replay.body.error.code).toBe('INSUFFICIENT_STOCK')
  expect(await read()).toEqual(after)
  await page.getByRole('link', { name: 'Аналитика', exact: true }).click()
  await page.getByRole('button', { name: 'Уйти без сохранения', exact: true }).click()
  await page.getByRole('navigation', { name: 'Отчёты аналитики' }).getByRole('link', { name: 'Недостача ингредиентов', exact: true }).click()
  await ready(page, 2); await expect(records(page)).toContainText('100 мл')
})
