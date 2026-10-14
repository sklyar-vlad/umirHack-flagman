import { randomUUID } from 'node:crypto'
import { expect, test, type APIRequestContext, type Page } from '@playwright/test'

declare global {
  interface Window {
    acceptanceStreamOpen: () => void
    acceptanceInventoryEvent: () => void
  }
}

const origin = process.env.FRONTEND_TEST_ORIGIN!
const password = process.env.FRONTEND_TEST_PASSWORD!
const emailForRole = (role: string) => `${role === 'order_operator' ? 'operator' : role}@flagman.invalid`
type RecordWithId = { id: string }
type Product = RecordWithId & { name: string }
type Batch = RecordWithId & { remaining_quantity: string }
type Supply = RecordWithId & { items: { batch: Batch }[] }
type Order = RecordWithId & { items: { allocations: { batch_id: string; quantity: string }[] }[] }
type List<T> = { items: T[]; total: number }
const observations = new WeakMap<Page, string[]>()

class Client {
  private csrf = ''
  private request: APIRequestContext
  constructor(request: APIRequestContext) { this.request = request }
  async login(role: string) {
    const response = await this.request.post('/api/v1/auth/login', { headers: { Origin: origin }, data: { email: emailForRole(role), password } })
    expect(response.status()).toBe(200)
    this.csrf = (await response.json() as { csrf_token: string }).csrf_token
  }
  async read<T>(path: string): Promise<T> {
    for (let attempt = 0; attempt < 3; attempt++) {
      const response = await this.request.get(`/api/v1${path}`)
      if (response.status() === 429 && attempt < 2) {
        const seconds = Number(response.headers()['retry-after'])
        expect(Number.isFinite(seconds) && seconds > 0 && seconds <= 5).toBe(true)
        await new Promise((resolve) => setTimeout(resolve, seconds * 1000))
        continue
      }
      expect(response.status()).toBe(200)
      return response.json() as Promise<T>
    }
    throw new Error('Read retry budget exhausted')
  }
  async write<T>(path: string, data: unknown, status = 201, method = 'POST'): Promise<T> {
    const response = await this.request.fetch(`/api/v1${path}`, { method, headers: { Origin: origin, 'X-CSRF-Token': this.csrf, 'Idempotency-Key': randomUUID() }, data })
    expect(response.status()).toBe(status)
    return response.json() as Promise<T>
  }
}
async function login(page: Page, role = 'manager', target = '/supplies/new') {
  await page.goto(`/login?returnTo=${encodeURIComponent(target)}`)
  await page.getByLabel(/^Email/).fill(emailForRole(role))
  await page.getByLabel(/^Пароль/).fill(password)
  await page.getByRole('button', { name: 'Войти', exact: true }).click()
  await expect(page).toHaveURL(target)
  await expect(page.getByText('Моковый режим', { exact: true })).toHaveCount(0)
  expect(await page.evaluate(() => navigator.serviceWorker.getRegistrations().then((values) => values.length))).toBe(0)
}
async function loseSuccessfulResponse(page: Page, path: string) {
  const attempts: { key: string | undefined; body: string | null }[] = []
  let handled = 0
  let saved: unknown
  await page.route(`**/api/v1${path}`, async (route) => {
    if (route.request().method() !== 'POST') { await route.continue(); return }
    attempts.push({ key: route.request().headers()['idempotency-key'], body: route.request().postData() })
    const response = await route.fetch()
    if (attempts.length === 1) {
      expect(response.status()).toBe(201)
      saved = await response.json()
      await route.abort('connectionreset')
    } else await route.fulfill({ response })
    handled++
  })
  return {
    async retry() {
      await page.getByRole('button', { name: 'Повторить исходный запрос', exact: true }).click()
      await expect.poll(() => handled).toBe(2)
      expect(attempts[1]).toEqual(attempts[0])
      expect(attempts[0].key).toMatch(/^[a-f\d-]{36}$/)
      await page.unroute(`**/api/v1${path}`)
      return saved
    },
  }
}

test.beforeEach(async ({ request, page }) => {
  // The real server allows 10 requests/sec with a burst of 20. Refill its global
  // bucket between independent UI scenarios rather than disabling rate limits.
  await new Promise((resolve) => setTimeout(resolve, 2_000))
  const statuses: string[] = []
  observations.set(page, statuses)
  page.on('response', (response) => { const path = new URL(response.url()).pathname; if (path.startsWith('/api/')) statuses.push(`${response.request().method()} ${path} ${response.status()}`) })
  page.on('pageerror', (error) => statuses.push(`JS: ${error.message}`))
  page.on('dialog', (dialog) => { if (dialog.type() === 'beforeunload') void dialog.accept(); else void dialog.dismiss() })
  await expect.poll(async () => {
    try { return (await request.get('/api/v1/ready')).status() } catch { return 0 }
  }, { timeout: 30_000 }).toBe(200)
})
test.afterEach(async ({ page }, info) => {
  if (info.status !== info.expectedStatus) console.log('Safe API status evidence:', observations.get(page))
})

test('real nginx: reception → two Margaritas → FEFO → write-off → atomic shortage; exact lost-response replay', async ({ page, playwright }, testInfo) => {
  const context = await playwright.request.newContext({ baseURL: origin })
  try {
    const admin = new Client(context)
    await admin.login('admin')
    const suffix = randomUUID().slice(0, 8)
    const category = await admin.write<RecordWithId>('/product-categories', { name: `Приёмка ${suffix}` })
    const products: Product[] = []
    for (const name of ['Тесто', 'Томаты', 'Сыр']) products.push(await admin.write<Product>('/products', { name: `${name} ${suffix}`, category_id: category.id, unit: 'g', display_unit: 'g', target_stock: '1000', standard_shelf_life_hours: 24 }))
    const dishName = `Маргарита ${suffix}`
    const dish = await admin.write<RecordWithId>('/dishes', { name: dishName, recipe: products.map((product, index) => ({ product_id: product.id, quantity_per_portion: index === 1 ? '50' : '100' })) })
    let opened = 0, notifications = 0
    await page.exposeFunction('acceptanceStreamOpen', () => { opened++ })
    await page.exposeFunction('acceptanceInventoryEvent', () => { notifications++ })
    await page.addInitScript(() => {
      const NativeSource = window.EventSource
      window.EventSource = class extends NativeSource {
        constructor(url: string | URL, options?: EventSourceInit) {
          super(url, options)
          this.addEventListener('open', () => window.acceptanceStreamOpen())
          this.addEventListener('inventory.changed', () => window.acceptanceInventoryEvent())
        }
      }
    })
    await login(page)
    await expect.poll(() => opened).toBeGreaterThan(0)
    const cookies = await page.context().cookies()
    expect(cookies.some((cookie) => cookie.httpOnly && cookie.sameSite === 'Lax')).toBe(true)
    await page.getByLabel(/^Номер поставки/).fill(`F31-${suffix}`)
    await page.getByLabel(/^Дата приёмки/).fill(new Date(Date.now() - 60_000 + 3 * 3_600_000).toISOString().slice(0, 19))
    const rows = [{ product: products[0], quantity: '150', days: 1 }, { product: products[0], quantity: '450', days: 2 }, { product: products[1], quantity: '200', days: 2 }, { product: products[2], quantity: '400', days: 2 }]
    for (const [index, row] of rows.entries()) {
      if (index) await page.getByRole('button', { name: 'Добавить строку' }).click()
      await page.getByLabel(`Продукт, строка ${index + 1}`, { exact: false }).selectOption(row.product.id)
      await page.getByLabel(`Количество, строка ${index + 1}`, { exact: false }).fill(row.quantity)
      // Restaurant local time far enough in the future; production server owns expiry.
      const date = new Date(Date.now() + row.days * 86_400_000 + 3 * 3_600_000).toISOString().slice(0, 16)
      await page.getByLabel(`Годен до, строка ${index + 1}`, { exact: false }).fill(date)
    }
    const receiptReplay = await loseSuccessfulResponse(page, '/supplies')
    await page.getByRole('button', { name: 'Принять поставку' }).click()
    await expect(page.getByRole('button', { name: 'Повторить исходный запрос' })).toBeVisible()
    await page.reload()
    const supply = await receiptReplay.retry() as Supply
    await expect(page.getByRole('heading', { name: 'Поставка принята' })).toBeVisible()
    const reads = new Client(page.request)
    await expect.poll(() => notifications).toBeGreaterThan(0)
    expect((await reads.read<List<RecordWithId>>(`/supplies?search=F31-${suffix}`)).total).toBe(1)
    expect((await reads.read<List<Batch>>(`/products/${products[0].id}/batches?status=all`)).items.map((batch) => batch.remaining_quantity)).toEqual(['150.000', '450.000'])
    // Authenticated cookie without CSRF must fail at the same-origin server.
    const missingCsrf = await page.request.post('/api/v1/write-offs', { headers: { Origin: origin, 'Idempotency-Key': randomUUID() }, data: { batch_id: supply.items[0].batch.id, quantity: '1', reason: 'CSRF must reject this write' } })
    expect(missingCsrf.status()).toBe(403)
    await page.goto(`/orders/new?search=${encodeURIComponent(dishName)}`)
    const add = page.getByRole('button', { name: `Добавить ${dishName}`, exact: true })
    await add.focus(); await page.keyboard.press('Enter')
    await page.getByLabel(`Порции: ${dishName}`, { exact: false }).fill('2')
    const submit = page.getByRole('button', { name: 'Оформить заказ', exact: true })
    await expect(submit).toBeEnabled()
    const orderReplay = await loseSuccessfulResponse(page, '/orders')
    await submit.click()
    await expect(page.getByRole('button', { name: 'Повторить исходный запрос' })).toBeVisible()
    const order = await orderReplay.retry() as Order
    await expect(page.getByRole('heading', { name: 'Заказ создан' })).toBeVisible()
    const allocations = order.items.flatMap((item) => item.allocations)
    expect(allocations.filter((allocation) => [supply.items[0].batch.id, supply.items[1].batch.id].includes(allocation.batch_id)).map((allocation) => allocation.quantity)).toEqual(['150.000', '50.000'])
    expect((await reads.read<List<Batch>>(`/products/${products[0].id}/batches?status=all`)).items.map((batch) => batch.remaining_quantity)).toEqual(['0.000', '400.000'])
    expect((await reads.read<List<RecordWithId>>('/orders')).items.filter((item) => item.id === order.id)).toHaveLength(1)
    await page.goto(`/inventory?search=${encodeURIComponent(products[1].name)}`)
    await page.getByRole('button', { name: `Раскрыть партии ${products[1].name}`, exact: true }).click()
    await page.getByRole('button', { name: 'Списать партию · строка 3', exact: true }).click()
    await page.getByLabel(/^Количество списания/).fill('100')
    await page.getByLabel(/^Причина списания/).fill('F31: повреждение упаковки')
    const writeOffReplay = await loseSuccessfulResponse(page, '/write-offs')
    await page.getByRole('button', { name: 'Списать', exact: true }).click()
    await page.getByRole('button', { name: 'Подтвердить списание', exact: true }).click()
    await expect(page.getByRole('button', { name: 'Повторить исходный запрос' })).toBeVisible()
    await writeOffReplay.retry()
    await expect(page.getByRole('dialog', { name: 'Списание выполнено' })).toBeVisible()
    await page.getByRole('button', { name: 'Готово', exact: true }).click()
    expect((await reads.read<List<RecordWithId>>(`/stock-movements?batch_id=${supply.items[2].batch.id}&type=manual_writeoff`)).total).toBe(1)
    await page.goto(`/stop-list?search=${encodeURIComponent(dishName)}`)
    await expect(page.getByRole('article').filter({ hasText: dishName })).toBeVisible()
    await page.goto(`/recommendations/procurement?search=${encodeURIComponent(products[1].name)}`)
    await expect(page.getByRole('article', { name: `Закупить: ${products[1].name}` })).toContainText('К закупке: 1000 г')
    const balances = async () => Promise.all(products.map(async (product) => (await reads.read<List<Batch>>(`/products/${product.id}/batches?status=all`)).items.map(({ id, remaining_quantity }) => ({ id, remaining_quantity }))))
    const before = await balances()
    const movementCount = (await reads.read<List<RecordWithId>>('/stock-movements')).total
    await page.goto(`/orders/new?search=${encodeURIComponent(dishName)}`)
    await page.getByRole('button', { name: `Добавить ${dishName}`, exact: true }).click()
    await page.getByLabel(`Порции: ${dishName}`, { exact: false }).fill('2')
    await expect(page.getByText('Корзину нельзя оформить.', { exact: true })).toBeVisible()
    await expect(submit).toBeDisabled()
    // Final POST tests server atomic refusal independently of the non-reserving check.
    const managerContext = await playwright.request.newContext({ baseURL: origin })
    try {
      const manager = new Client(managerContext); await manager.login('manager')
      const refusal = await manager.write<{ error: { code: string } }>('/orders', { items: [{ dish_id: dish.id, portions: 2, recipe_version: 1 }] }, 409)
      expect(refusal.error.code).toBe('INSUFFICIENT_STOCK')
    } finally { await managerContext.dispose() }
    await expect(page.getByLabel(`Порции: ${dishName}`, { exact: false })).toHaveValue('2')
    expect(await balances()).toEqual(before)
    expect((await reads.read<List<RecordWithId>>('/stock-movements')).total).toBe(movementCount)
    await page.screenshot({ path: testInfo.outputPath('real-basket-shortage.png'), fullPage: true })
  } finally { await context.dispose() }
})

for (const [role, width, blocked] of [
  ['admin', 1440, null], ['manager', 1024, '/admin/users'], ['storekeeper', 390, '/orders'], ['order_operator', 360, '/supplies'],
] as const) test(`real nginx ${role}: nested reload, role guard and ${width}px`, async ({ page }) => {
  await page.setViewportSize({ width, height: 1000 })
  await login(page, role, '/inventory?search=nonexistent-f31')
  await expect(page.getByRole('heading', { name: 'Склад', exact: true })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Склад', exact: true })).toBeVisible()
  if (blocked) {
    const business: string[] = []
    page.on('request', (request) => { const path = new URL(request.url()).pathname; if (path.startsWith('/api/v1/') && !path.startsWith('/api/v1/auth/') && path !== '/api/v1/events') business.push(path) })
    await page.goto(blocked)
    await expect(page.getByRole('heading', { name: 'Недостаточно прав' })).toBeVisible()
    expect(business).toEqual([])
  } else {
    await page.goto('/admin/users?page=1')
    await page.reload()
    await expect(page.getByRole('heading', { name: 'Пользователи и роли', exact: true })).toBeVisible()
  }
  // This case already reloaded inventory and administration. Refill the real
  // shared API bucket before the additional catalogue/detail/reload sequence.
  await new Promise((resolve) => setTimeout(resolve, 2_000))
  await page.goto('/dishes')
  if (role === 'storekeeper') {
    await expect(page.getByRole('heading', { name: 'Недостаточно прав' })).toBeVisible()
  } else {
    await expect(page.getByRole('heading', { name: 'Рецептуры', exact: true })).toBeVisible()
    const first = page.getByRole('article', { name: /^Рецептура:/ }).first()
    await expect(first).toBeVisible()
    await first.getByRole('link', { name: /^Открыть рецептуру/ }).click()
    await expect(page.getByRole('region', { name: 'Состав на одну порцию' })).toBeVisible()
    await page.reload()
    await expect(page.getByRole('region', { name: 'Состав на одну порцию' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'Редактировать рецептуру' })).toHaveCount(role === 'admin' ? 1 : 0)
  }
  if (width < 768) {
    await page.getByRole('button', { name: 'Открыть меню' }).click()
    const box = (await page.getByRole('dialog', { name: 'Меню', exact: true }).boundingBox())!
    expect(box.x).toBe(0); expect(box.width).toBe(320)
    await page.keyboard.press('Escape')
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.getByRole('button', { name: 'Выйти', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Вход в Кладовую' })).toBeVisible()
  expect((await page.request.get('/api/v1/auth/me')).status()).toBe(401)
})

test('real sessions: expiration restores only the same owner and another user cannot read the draft', async ({ page }) => {
  await login(page)
  const number = page.getByLabel(/^Номер поставки/)
  await number.fill('F31-PRIVATE-MANAGER-DRAFT')
  await page.context().clearCookies()
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Вход в Кладовую' })).toBeVisible()
  await login(page)
  await expect(number).toHaveValue('F31-PRIVATE-MANAGER-DRAFT')
  await page.context().clearCookies()
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Вход в Кладовую' })).toBeVisible()
  await login(page, 'admin')
  await expect(number).toHaveValue('')
  expect(await page.evaluate(() => sessionStorage.getItem('flagman.reception.v1')?.includes('F31-PRIVATE-MANAGER-DRAFT') ?? false)).toBe(false)
})
