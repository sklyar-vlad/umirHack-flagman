import { backgroundRefresh } from './refresh'
import { test, expect, type Page } from '@playwright/test'
async function login(page: Page, target = '/recommendations/procurement', role = 'manager') {
  await page.goto(`/login?returnTo=${encodeURIComponent(target)}`)
  if (role !== 'manager') await page.getByLabel('Сценарий API').selectOption(`role-${role.replaceAll('_', '-')}`)
  await page.getByLabel(/^Email/).fill('employee@example.test'); await page.getByLabel(/^Пароль/).fill('local-test-only')
  await page.getByRole('button', { name: 'Войти', exact: true }).click()
}
const milk = (page: Page) => page.getByRole('article', { name: 'Закупить: Молоко' })
for (const width of [360, 390, 1024, 1440]) {
  test(`procurement priority, exact units, keyboard, filters and links at ${width}px`, async ({ page }) => {
    const errors: string[] = [], reads: URL[] = [], writes: string[] = []
    page.on('pageerror', (error) => errors.push(error.message))
    page.on('request', (request) => {
      if (new URL(request.url()).pathname === '/api/v1/procurement-recommendations') reads.push(new URL(request.url()))
      if (request.method() === 'POST' && !request.url().includes('/auth/')) writes.push(request.url())
    })
    await page.setViewportSize({ width, height: 1000 }); await login(page, '/recommendations?tab=procurement')
    await expect(milk(page)).toBeVisible(); await expect(page.getByText(/Продуктов к закупке: 23. Страница 1/)).toBeVisible()
    await expect(page.getByRole('article').first()).toHaveAccessibleName('Закупить: Молоко')
    for (const value of ['1,5 л', '4 л', '2,5 л', '0,2 л', 'Ниже целевого запаса']) await expect(milk(page)).toContainText(value)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: `test-results/procurement-${width}.png`, fullPage: true })
    await expect(page.getByRole('button', { name: 'Следующая страница' })).toBeEnabled()
    await page.getByRole('button', { name: 'Следующая страница' }).focus(); await page.keyboard.press('Enter')
    await expect(page.getByText(/Страница 2. Снимок/)).toBeVisible(); await expect(milk(page)).toHaveCount(0)
    await expect(page.getByRole('article', { name: 'Закупить: Яйцо' })).toContainText('10 шт.')
    const expired = page.getByRole('article', { name: 'Закупить: Творог — только просрочка' })
    await expect(expired).toContainText('Остались только просроченные партии'); await expect(expired).toContainText('3 кг')
    await page.getByLabel('Поиск продукта').fill('МОЛОКО'); await page.getByLabel('Поиск продукта').press('Enter')
    await expect(milk(page)).toBeVisible(); await expect(page.getByRole('article')).toHaveCount(1)
    expect(new URL(page.url()).searchParams.has('page')).toBe(false)
    await page.getByLabel('Категория', { exact: true }).selectOption({ label: 'Штучные продукты' }); await expect(page.getByRole('heading', { name: 'Ничего не найдено' })).toBeVisible()
    await page.goBack(); await expect(milk(page)).toBeVisible()
    await page.goBack(); await expect(page.getByText(/Страница 2. Снимок/)).toBeVisible()
    await page.goForward(); await expect(milk(page)).toBeVisible()
    await milk(page).getByRole('link', { name: 'Молочный суп' }).focus(); await page.keyboard.press('Enter')
    await expect(page.getByRole('article', { name: 'Блюдо: Молочный суп' })).toBeVisible()
    await page.goBack(); await expect(milk(page)).toBeVisible()
    await milk(page).getByRole('link', { name: 'На склад: Молоко' }).click(); await expect(page.getByRole('heading', { name: 'Молоко', exact: true })).toBeVisible()
    expect(new URL(page.url()).searchParams.get('product_id')).toBe('00000000-0000-4000-8000-000000000010')
    await page.goBack(); await expect(milk(page)).toBeVisible()
    await milk(page).getByRole('link', { name: 'Принять поставку: Молоко' }).focus(); await page.keyboard.press('Enter')
    await expect(page.getByLabel('Продукт, строка 1', { exact: false })).toHaveValue('00000000-0000-4000-8000-000000000010')
    await expect(page.getByLabel('Единица, строка 1')).toHaveValue('l'); await expect(page.getByLabel('Количество, строка 1', { exact: false })).toHaveValue('')
    expect(writes).toEqual([])
    await page.reload(); await expect(page.getByLabel('Продукт, строка 1', { exact: false })).toHaveValue('00000000-0000-4000-8000-000000000010')
    expect(reads.every((url) => [...url.searchParams.keys()].every((key) => ['search', 'category_id', 'page', 'page_size'].includes(key)))).toBe(true)
    expect(errors).toEqual([])
  })
}
test('loading, stale/error, category retry, empty and denial states', async ({ page }) => {
  await login(page); await expect(milk(page)).toBeVisible()
  await backgroundRefresh(page, 'slow', '/api/v1/procurement-recommendations', 'Загрузка закупок', page.getByRole('article').first(), page.getByRole('button', { name: 'Обновить закупки' }))
  await page.getByLabel('Сценарий API').selectOption('categories-error'); await expect(page.getByRole('heading', { name: 'Не удалось загрузить категории' })).toBeVisible(); await expect(milk(page)).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('success'); await expect(page.getByLabel('Категория', { exact: true })).toBeEnabled()
  await page.getByLabel('Сценарий API').selectOption('network-error'); await expect(page.getByText(/Данные могут быть устаревшими/)).toBeVisible(); await expect(milk(page)).toBeVisible()
  await page.getByLabel('Поиск продукта').fill('%_\\'); await page.getByLabel('Поиск продукта').press('Enter'); await expect(page.getByRole('article')).toHaveCount(0)
  await page.evaluate(async () => { const path = '/src/app/mocks/scenarios.ts'; const { setMockScenario } = await import(path); setMockScenario('success') })
  const retry = page.getByRole('alert').filter({ has: page.getByRole('heading', { name: 'Не удалось загрузить закупки' }) }).getByRole('button', { name: 'Попробовать снова' })
  await retry.focus(); await page.keyboard.press('Enter'); await expect(page.getByRole('heading', { name: 'Ничего не найдено' })).toBeVisible()
  await page.getByRole('search').getByRole('button', { name: 'Сбросить фильтры' }).click(); await expect(milk(page)).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('procurement-empty'); await expect(page.getByRole('heading', { name: 'Закупки не требуются' })).toBeVisible(); await expect(page.getByText(/Продукты без цели/)).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('success'); await expect(milk(page)).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('forbidden'); await expect(page.getByRole('article')).toHaveCount(0); await expect(page.getByRole('heading', { name: 'Недостаточно прав' }).last()).toBeVisible()
})
for (const role of ['admin', 'storekeeper']) {
  test(`${role} procurement links follow roles`, async ({ page }) => {
    await login(page, '/recommendations/procurement?search=Молоко', role); await expect(milk(page)).toBeVisible()
    await expect(milk(page).getByRole('link', { name: 'Молочный суп' })).toHaveCount(role === 'admin' ? 1 : 0)
    await expect(milk(page)).toContainText('Молочный суп')
    await expect(page.getByRole('navigation', { name: 'Вкладки рекомендаций' }).getByRole('link', { name: 'Продвигать блюда' })).toHaveCount(role === 'admin' ? 1 : 0)
    await expect(milk(page).getByRole('link', { name: 'Принять поставку: Молоко' })).toBeVisible()
  })
}
test('operator cannot mount procurement from either route', async ({ page }) => {
  const reads: string[] = []; page.on('request', (request) => { if (new URL(request.url()).pathname === '/api/v1/procurement-recommendations') reads.push(request.url()) })
  await login(page, '/recommendations/procurement', 'order_operator'); await expect(page.getByRole('heading', { name: 'Обзор' })).toBeVisible()
  for (const path of ['/recommendations/procurement', '/recommendations?tab=procurement']) {
    await page.goto(path); await expect(page.getByRole('heading', { name: 'Недостаточно прав' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'Закупить продукты', exact: true })).toHaveCount(0)
  }
  expect(reads).toEqual([])
})
test('reception reduces procurement and a confirmed order increases it', async ({ page }) => {
  await login(page, '/recommendations/procurement?search=Молоко'); await expect(milk(page)).toContainText('К закупке: 2,5 л')
  await milk(page).getByRole('link', { name: 'Принять поставку: Молоко' }).click()
  await expect(page.getByLabel('Продукт, строка 1', { exact: false })).toHaveValue('00000000-0000-4000-8000-000000000010')
  await page.getByLabel(/^Номер поставки/).fill('F13-БРАУЗЕР'); await page.getByLabel(/^Дата приёмки/).fill('2098-01-01T12:00')
  await page.getByLabel('Количество, строка 1', { exact: false }).fill('1,000001'); await page.getByLabel('Годен до, строка 1', { exact: false }).fill('2099-01-01T12:00')
  await page.getByRole('button', { name: 'Принять поставку', exact: true }).click(); await expect(page.getByRole('heading', { name: 'Поставка принята' })).toBeVisible()
  await page.goto('/recommendations/procurement?search=Молоко'); await expect(milk(page)).toContainText('К закупке: 1,499999 л'); await expect(milk(page).getByRole('link', { name: 'Молочный суп' })).toHaveCount(0)
  await page.goto('/orders/new'); await page.getByRole('button', { name: 'Добавить Суп дня', exact: true }).click()
  const submit = page.getByRole('button', { name: 'Оформить заказ', exact: true }); await expect(submit).toBeEnabled(); await submit.click(); await expect(page.getByRole('heading', { name: 'Заказ №1001' })).toBeVisible()
  await page.goto('/recommendations/procurement?search=Молоко'); await expect(milk(page)).toContainText('К закупке: 1,599999 л')
})
test('time event updates expiration without a mutation', async ({ page }) => {
  await page.clock.install(); await login(page, '/inventory?search=Молоко')
  await expect(page.getByRole('article', { name: 'Молоко' })).toBeVisible()
  // The warehouse snapshot supplies the known expiry boundary for this time event.
  await page.evaluate(async () => {
    const path = '/src/app/mocks/inventory-fixtures.ts'
    const { inventoryItems } = await import(path)
    inventoryItems[0].nearest_expires_at = new Date(Date.now() + 20_000).toISOString()
    inventoryItems[0].remaining_seconds = 20
  })
  const refreshed = page.waitForResponse((response) => new URL(response.url()).pathname === '/api/v1/inventory')
  await page.getByRole('button', { name: 'Обновить склад' }).click()
  await refreshed
  await expect(page.getByRole('button', { name: 'Обновить склад' })).toBeEnabled()
  await page.getByRole('navigation', { name: 'Разделы приложения', exact: true }).getByRole('link', { name: 'Рекомендации', exact: true }).click()
  await page.getByRole('navigation', { name: 'Вкладки рекомендаций' }).getByRole('link', { name: 'Закупить продукты', exact: true }).click()
  await expect(milk(page)).toContainText('К закупке: 2,5 л')
  await page.evaluate(async () => { const path = '/src/app/mocks/scenarios.ts'; const { setMockScenario } = await import(path); setMockScenario('procurement-expired') })
  await page.clock.runFor(30_500); await expect(milk(page)).toContainText('К закупке: 4 л'); await expect(milk(page)).toContainText('Нет годного остатка')
})
