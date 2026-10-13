import { expect, test, type Page } from '@playwright/test'
import { createServer } from 'node:http'

type Observations = { sources: number; closed: number; opened: number; deliveries: string[] }
type ObservedWindow = Window & { __liveTest: Observations }
async function observe(page: Page) {
  await page.addInitScript(() => {
    const NativeSource = window.EventSource
    const observations = { sources: 0, closed: 0, opened: 0, deliveries: [] as string[] }
    ;(window as ObservedWindow).__liveTest = observations
    window.EventSource = class extends NativeSource {
      constructor(url: string | URL, options?: EventSourceInit) {
        super(url, options); observations.sources++
        this.addEventListener('open', () => { observations.opened++ })
        for (const type of ['inventory.time_changed', 'resync.required']) this.addEventListener(type, (event) => { observations.deliveries.push(`${type}:${(event as MessageEvent).lastEventId}`) })
      }
      close() { observations.closed++; super.close() }
    }
  })
}
const observed = (page: Page) => page.evaluate(() => (window as ObservedWindow).__liveTest)
async function login(page: Page, target = '/orders/new', admin = false) {
  await page.goto(`/login?returnTo=${encodeURIComponent(target)}`)
  if (admin) await page.getByLabel('Сценарий API').selectOption('role-admin')
  await page.getByLabel(/^Email/).fill('employee@example.test'); await page.getByLabel(/^Пароль/).fill('local-test-only')
  await page.getByRole('button', { name: 'Войти', exact: true }).click()
  await expect.poll(async () => (await observed(page)).opened).toBeGreaterThan(0)
}
async function scenario(page: Page, value: string) {
  await page.evaluate((value) => { const select = document.querySelector<HTMLSelectElement>('#mock-scenario')!; select.value = value; select.dispatchEvent(new Event('change', { bubbles: true })) }, value)
}
for (const width of [360, 390, 1024, 1440]) test(`F30 native replay, duplicate cursor, resync and basket preserved at ${width}px`, async ({ page }) => {
  let checks = 0
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  page.on('request', (request) => { if (new URL(request.url()).pathname === '/api/v1/orders/check') checks++ })
  await page.setViewportSize({ width, height: 1000 }); await observe(page); await login(page)
  const add = page.getByRole('button', { name: 'Добавить Суп дня', exact: true })
  await add.focus(); await page.keyboard.press('Enter')
  const portions = page.getByLabel(/^Порции: Суп дня/), submit = page.getByRole('button', { name: 'Оформить заказ', exact: true })
  await portions.fill('2'); await expect(submit).toBeEnabled()
  const before = checks
  await scenario(page, 'events-replay')
  await expect.poll(async () => (await observed(page)).deliveries.length).toBeGreaterThan(0)
  await expect.poll(() => checks).toBeGreaterThan(before)
  await expect(submit).toBeEnabled(); const refreshed = checks
  await expect.poll(async () => (await observed(page)).deliveries.length).toBe(2)
  await page.waitForTimeout(650)
  expect(checks).toBe(refreshed); await expect(portions).toHaveValue('2')
  const deliveries = (await observed(page)).deliveries
  expect(deliveries[0]).toBe(deliveries[1])
  await scenario(page, 'events-resync')
  await expect.poll(async () => (await observed(page)).deliveries.some((value) => value.startsWith('resync.required:'))).toBe(true)
  await expect.poll(() => checks).toBeGreaterThan(refreshed)
  await expect(submit).toBeEnabled(); await expect(portions).toHaveValue('2')
  expect((await observed(page)).sources - (await observed(page)).closed).toBe(1)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: `test-results/live-updates-${width}.png`, fullPage: true })
  expect(errors).toEqual([])
})
test('F30 external notifications retain dirty product values and detached revision', async ({ page }) => {
  let details = 0
  page.on('request', (request) => { if (new URL(request.url()).pathname === '/api/v1/products/00000000-0000-4000-8000-000000000010') details++ })
  await observe(page); await login(page, '/admin/products/00000000-0000-4000-8000-000000000010/edit', true)
  const name = page.getByLabel('Название продукта', { exact: true })
  await name.fill('Незавершённое редактирование')
  const before = details
  await scenario(page, 'events-resync')
  await expect.poll(async () => (await observed(page)).deliveries.some((value) => value.startsWith('resync.required:'))).toBe(true)
  await page.waitForTimeout(600)
  await expect(name).toHaveValue('Незавершённое редактирование'); expect(details).toBe(before)
})
test('F30 manager reception choices refresh on resync without losing entered values', async ({ page }) => {
  let products = 0
  page.on('request', (request) => { if (new URL(request.url()).pathname === '/api/v1/products') products++ })
  await observe(page); await login(page, '/supplies/new')
  const number = page.getByLabel(/^Номер поставки/), quantity = page.getByLabel('Количество, строка 1', { exact: false })
  const product = page.getByLabel('Продукт, строка 1', { exact: false })
  await expect(product).toContainText('Молоко')
  await number.fill('F30-НЕСОХРАНЁННАЯ')
  await product.selectOption('00000000-0000-4000-8000-000000000010')
  await quantity.fill('0,5')
  const before = products
  await scenario(page, 'events-resync')
  await expect.poll(async () => (await observed(page)).deliveries.some((value) => value.startsWith('resync.required:'))).toBe(true)
  await expect.poll(() => products).toBeGreaterThan(before)
  await expect(number).toHaveValue('F30-НЕСОХРАНЁННАЯ')
  await expect(product).toHaveValue('00000000-0000-4000-8000-000000000010')
  await expect(quantity).toHaveValue('0,5')
})
test('F30 session revoke confirms me, closes stream and allows same-owner basket restoration', async ({ page }) => {
  await observe(page); await login(page)
  await page.getByRole('button', { name: 'Добавить Суп дня', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Оформить заказ', exact: true })).toBeEnabled()
  await scenario(page, 'session-expired')
  await page.evaluate(() => window.dispatchEvent(new Event('focus')))
  await expect(page.getByRole('heading', { name: 'Вход в Кладовую' })).toBeVisible()
  expect((await observed(page)).sources).toBe((await observed(page)).closed)
  await scenario(page, 'success')
  await page.getByLabel(/^Email/).fill('employee@example.test'); await page.getByLabel(/^Пароль/).fill('local-test-only'); await page.getByRole('button', { name: 'Войти', exact: true }).click()
  await expect(page.getByLabel(/^Порции: Суп дня/)).toHaveValue('1')
})
test('F30 online recovery refreshes the active snapshot without duplicate subscriptions', async ({ page, context }) => {
  let reads = 0
  page.on('request', (request) => { if (new URL(request.url()).pathname === '/api/v1/inventory') reads++ })
  await observe(page); await login(page, '/inventory')
  await expect(page.getByRole('heading', { name: 'Склад', exact: true })).toBeVisible()
  await page.waitForTimeout(250); const before = reads
  await context.setOffline(true); await page.waitForTimeout(250)
  await context.setOffline(false)
  await expect.poll(() => reads).toBeGreaterThan(before)
  const counters = await observed(page); expect(counters.sources - counters.closed).toBe(1)
})
test('F30 native reconnect sends Last-Event-ID and refreshes on the same controller', async ({ page }) => {
  // Isolate native transport from the service worker; other tests exercise MSW + REST.
  const headers: string[] = [], urls: string[] = []
  const server = createServer((request, response) => {
    if (request.url === '/native-sse.html') { response.writeHead(200, { 'Content-Type': 'text/html' }); response.end('<!doctype html><title>Local SSE transport fixture</title>'); return }
    urls.push(request.url ?? '')
    const cursor = request.headers['last-event-id']
    if (typeof cursor === 'string') headers.push(cursor)
    response.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache' })
    response.write(`id: ${typeof cursor === 'string' ? BigInt(cursor) + 1n : 9007199254740993n}\nevent: inventory.time_changed\ndata: ${JSON.stringify({ occurred_at: new Date().toISOString(), topics: ['inventory'] })}\n\n`)
    const timer = setTimeout(() => response.end(), 1_000)
    response.on('close', () => clearTimeout(timer))
  })
  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve))
  const address = server.address()
  if (!address || typeof address === 'string') throw new Error('Local SSE fixture did not start')
  try {
    await page.goto(`http://127.0.0.1:${address.port}/native-sse.html`)
    await page.evaluate(async () => {
      const path = 'http://127.0.0.1:5182/src/shared/api/live-events.ts'
      const { createLiveEvents } = await import(path)
      const state = { sources: 0, refreshes: 0, cursors: [] as string[], stop: () => {} }
      ;(window as Window & { __transportTest: typeof state }).__transportTest = state
      const live = createLiveEvents({
        connect: () => { state.sources++; return new EventSource('/events', { withCredentials: true }) },
        visible: () => true, online: () => true, confirmSession: async () => true,
        refresh: (events: { cursor: bigint }[]) => { state.refreshes++; state.cursors.push(...events.map((event) => event.cursor.toString())) },
      })
      state.stop = live.stop
    })
    await expect.poll(() => headers, { timeout: 15_000 }).toContain('9007199254740993')
    await expect.poll(() => page.evaluate(() => (window as Window & { __transportTest: { cursors: string[] } }).__transportTest.cursors)).toContain('9007199254740994')
    const state = await page.evaluate(() => { const state = (window as Window & { __transportTest: { sources: number; refreshes: number; stop: () => void } }).__transportTest; state.stop(); return { sources: state.sources, refreshes: state.refreshes } })
    expect(state.sources).toBe(1); expect(state.refreshes).toBeGreaterThanOrEqual(2)
    expect(urls.every((url) => url === '/events')).toBe(true)
  } finally { server.closeAllConnections(); await new Promise<void>((resolve) => server.close(() => resolve())) }
})
