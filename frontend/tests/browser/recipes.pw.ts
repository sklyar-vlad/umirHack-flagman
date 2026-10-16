import { expect, test, type Page } from '@playwright/test'
async function login(page: Page, role = 'manager', target = '/dishes') {
  await page.goto(`/login?returnTo=${encodeURIComponent(target)}`)
  if (role !== 'manager') await page.getByLabel('Сценарий API').selectOption(`role-${role === 'order_operator' ? 'order-operator' : role}`)
  await page.getByLabel(/^Email/).fill('employee@example.test')
  await page.getByLabel(/^Пароль/).fill('local-test-only')
  await page.getByRole('button', { name: 'Войти', exact: true }).click()
}
test('menu keeps adding dishes to an order alongside the recipe catalogue', async ({ page }) => {
  await login(page, 'manager', '/menu')
  await expect(page.getByRole('heading', { name: 'Меню', exact: true })).toBeVisible()
  const dish = page.getByRole('article').filter({ has: page.getByRole('heading', { name: 'Суп дня', exact: true }) })
  await dish.getByRole('button', { name: 'Добавить в заказ' }).click()
  await expect(page).toHaveURL('/orders/new')
  await expect(page.getByLabel(/^Порции: Суп дня/)).toHaveValue('1')
})
for (const width of [360, 390, 1024, 1440]) test(`recipe catalogue and direct detail at ${width}px`, async ({ page }) => {
  await page.setViewportSize({ width, height: 1000 })
  const errors: string[] = [], writes: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  await login(page, 'manager', '/dishes?search=Суп')
  page.on('request', (request) => { if (request.method() !== 'GET' && request.url().includes('/api/v1/')) writes.push(request.url()) })
  await expect(page.getByRole('heading', { name: 'Рецептуры', exact: true })).toBeVisible()
  const card = page.getByRole('article', { name: 'Рецептура: Суп дня' })
  await expect(card).toContainText('100 мл')
  await expect(page.getByRole('link', { name: 'Управлять рецептурами' })).toHaveCount(0)
  if (width < 768) {
    await page.getByRole('button', { name: 'Открыть меню' }).click()
    const menu = page.getByRole('dialog', { name: 'Меню', exact: true })
    const box = (await menu.boundingBox())!
    expect(box.x).toBe(0); expect(box.width).toBe(320); expect(box.height).toBe(1000)
    await expect(menu.locator('[aria-current="page"]')).toHaveCount(1)
    await expect(menu.getByRole('link', { name: 'Рецептуры', exact: true })).toHaveAttribute('aria-current', 'page')
    await page.screenshot({ path: `test-results/recipes-menu-${width}.png`, fullPage: true })
    await page.keyboard.press('Escape')
    await expect(page.getByRole('button', { name: 'Открыть меню' })).toBeFocused()
  }
  await card.getByRole('link', { name: 'Открыть рецептуру', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Суп дня', exact: true })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Состав на одну порцию' })).toContainText('100 мл')
  await expect(page.getByRole('link', { name: 'Редактировать рецептуру' })).toHaveCount(0)
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Суп дня', exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: `test-results/recipe-detail-${width}.png`, fullPage: true })
  await page.getByRole('link', { name: 'К рецептурам', exact: false }).click()
  await expect(page.getByLabel('Поиск блюда')).toHaveValue('Суп')
  await page.getByLabel('Поиск блюда').fill('%_')
  await page.getByRole('button', { name: 'Найти', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Ничего не найдено' })).toBeVisible()
  await page.getByRole('button', { name: 'Сбросить фильтры' }).first().click()
  await expect(card).toBeVisible()
  expect(writes).toEqual([]); expect(errors).toEqual([])
})
for (const role of ['admin', 'order_operator', 'storekeeper']) test(`recipes respect ${role} permissions`, async ({ page }) => {
  await login(page, role)
  if (role === 'storekeeper') {
    const reads: string[] = []
    page.on('request', (request) => { if (new URL(request.url()).pathname.startsWith('/api/v1/dishes')) reads.push(request.url()) })
    await page.goto('/dishes')
    await expect(page.getByRole('heading', { name: 'Недостаточно прав' })).toBeVisible()
    expect(reads).toEqual([])
    return
  }
  const card = page.getByRole('article', { name: 'Рецептура: Суп дня' })
  await expect(card).toBeVisible()
  await expect(page.getByRole('link', { name: 'Управлять рецептурами' })).toHaveCount(role === 'admin' ? 1 : 0)
  await card.getByRole('link', { name: 'Открыть рецептуру', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Суп дня', exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Редактировать рецептуру' })).toHaveCount(role === 'admin' ? 1 : 0)
  if (role === 'admin') {
    await page.getByRole('link', { name: 'Редактировать рецептуру' }).click()
    await expect(page.getByRole('heading', { name: 'Редактирование блюда' })).toBeVisible()
  }
})
