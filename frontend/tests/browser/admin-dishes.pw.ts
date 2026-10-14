import { backgroundRefresh } from './refresh'
import { test, expect, type Page } from '@playwright/test'
async function login(page: Page, role = 'admin', target = '/admin/dishes') {
  await page.goto(`/login?returnTo=${encodeURIComponent(target)}`)
  if (role !== 'manager') await page.getByLabel('Сценарий API').selectOption(`role-${role.replaceAll('_', '-')}`)
  await page.getByLabel(/^Email/).fill('employee@example.test'); await page.getByLabel(/^Пароль/).fill('local-test-only'); await page.getByRole('button', { name: 'Войти', exact: true }).click()
}
for (const width of [360, 390, 1024, 1440]) {
  test(`F19 catalogue, fresh preview and keyboard at ${width}px`, async ({ page }) => {
    const errors: string[] = [], reads: URL[] = [], writes: string[] = []
    page.on('pageerror', (error) => errors.push(error.message))
    page.on('request', (request) => { const url = new URL(request.url()); if (url.pathname === '/api/v1/dishes') reads.push(url); if (/\/api\/v1\/(orders|dishes)/.test(url.pathname) && request.method() !== 'GET') writes.push(request.url()) })
    await page.setViewportSize({ width, height: 1000 }); await login(page)
    const soup = page.getByRole('article', { name: 'Блюдо: Суп дня' })
    await expect(soup).toContainText('Горячий суп'); await expect(soup).toContainText('Ингредиентов1')
    await expect(page.getByRole('article', { name: 'Блюдо: Отключённое блюдо' })).toContainText('Отключено')
    await expect(page.getByRole('article', { name: 'Блюдо: Без рецептуры' })).toContainText('Рецептура отсутствует')
    expect(Object.fromEntries(reads[0].searchParams)).toEqual({ availability: 'all', is_active: 'all', sort: 'name', page: '1', page_size: '50' })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: `test-results/f19-dishes-${width}.png`, fullPage: false }); await soup.screenshot({ path: `test-results/f19-dish-card-${width}.png` })
    const opener = page.getByRole('button', { name: 'Состав: Суп дня', exact: true }); await opener.focus(); await page.keyboard.press('Enter')
    const dialog = page.getByRole('dialog', { name: 'Состав: Суп дня' }); await expect(dialog).toContainText('Молоко — 100 мл на порцию')
    for (let step = 0; step < 5; step++) { await page.keyboard.press('Tab'); expect(await dialog.evaluate((element) => element.contains(document.activeElement))).toBe(true) }
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); await page.screenshot({ path: `test-results/f19-recipe-${width}.png`, fullPage: false })
    await page.keyboard.press('Escape'); await expect(dialog).toHaveCount(0); await expect(opener).toBeFocused()
    await page.getByRole('link', { name: 'Редактировать: Суп дня' }).focus(); await page.keyboard.press('Enter'); await expect(page.getByRole('heading', { name: 'Редактирование блюда' })).toBeVisible()
    expect(new URL(page.url()).pathname).toBe('/admin/dishes/00000000-0000-4000-8000-000000000800/edit')
    await page.getByRole('link', { name: 'В каталог блюд' }).click(); await expect(soup).toBeVisible()
    await page.getByRole('link', { name: 'Создать блюдо', exact: true }).click(); await expect(page.getByRole('heading', { name: 'Создание блюда' })).toBeVisible()
    expect(writes).toEqual([]); expect(errors).toEqual([])
  })
}
test('F19 URL filters, pagination, literal search, back/forward and alias', async ({ page }) => {
  await login(page); await expect(page.getByRole('article')).toHaveCount(25)
  await page.getByLabel('Блюд на странице').selectOption('20'); await expect(page.getByRole('article')).toHaveCount(20)
  await page.getByRole('button', { name: 'Страница 2', exact: true }).click(); await expect(page.getByRole('article')).toHaveCount(5)
  await page.getByLabel('Активность', { exact: true }).selectOption('false'); await expect(page.getByRole('article')).toHaveCount(1); expect(new URL(page.url()).searchParams.has('page')).toBe(false)
  await page.getByLabel('Доступность', { exact: true }).selectOption('available'); await expect(page.getByRole('heading', { name: 'Ничего не найдено' })).toBeVisible(); await expect(page.getByRole('article')).toHaveCount(0)
  await page.goBack(); await expect(page.getByRole('article', { name: 'Блюдо: Отключённое блюдо' })).toBeVisible(); await page.goForward(); await expect(page.getByRole('heading', { name: 'Ничего не найдено' })).toBeVisible()
  await page.getByLabel('Поиск блюда').fill('%_'); await page.getByRole('button', { name: 'Найти', exact: true }).click(); expect(new URL(page.url()).searchParams.get('search')).toBe('%_')
  await page.getByRole('button', { name: 'Сбросить фильтры' }).first().click(); await expect(page.getByRole('article')).toHaveCount(25)
  await page.goto('/admin/recipes?is_active=false'); await expect(page.getByRole('article')).toHaveCount(1)
  await page.getByRole('link', { name: 'Редактировать: Отключённое блюдо' }).click(); await page.getByRole('link', { name: 'В каталог блюд' }).click(); expect(new URL(page.url()).pathname).toBe('/admin/dishes'); expect(new URL(page.url()).searchParams.get('is_active')).toBe('false')
})
test('F19 loading, stale/retry, empty, isolated filters and access denial', async ({ page }) => {
  await login(page); await expect(page.getByRole('article')).toHaveCount(25)
  await backgroundRefresh(page, 'dishes-slow', '/api/v1/dishes', 'Загрузка каталога блюд', page.getByRole('button', { name: 'Состав: Суп дня', exact: true }), page.getByRole('button', { name: 'Обновить каталог' }))
  await page.getByLabel('Сценарий API').selectOption('dishes-error'); await expect(page.getByText(/Данные могут быть устаревшими/)).toBeVisible(); await expect(page.getByRole('article')).toHaveCount(25)
  await page.evaluate(async () => { const path = '/src/app/mocks/scenarios.ts'; const { setMockScenario } = await import(path); setMockScenario('success') })
  await page.getByRole('button', { name: 'Попробовать снова' }).focus(); await page.keyboard.press('Enter'); await expect(page.getByRole('heading', { name: 'Не удалось загрузить каталог блюд' })).toHaveCount(0)
  await page.getByLabel('Сценарий API').selectOption('dishes-empty'); await expect(page.getByRole('heading', { name: 'Каталог блюд пуст' })).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('dishes-error'); await page.getByLabel('Активность', { exact: true }).selectOption('false'); await expect(page.getByRole('heading', { name: 'Не удалось загрузить каталог блюд' })).toBeVisible(); await expect(page.getByRole('article')).toHaveCount(0)
  await page.getByLabel('Сценарий API').selectOption('success'); await expect(page.getByRole('article')).toHaveCount(1)
  await page.getByLabel('Сценарий API').selectOption('forbidden'); await expect(page.getByRole('heading', { name: 'Недостаточно прав' })).toBeVisible(); await expect(page.getByRole('article')).toHaveCount(0)
})
test('F19 new-version refresh and preview missing/error/loading hide cached ingredients', async ({ page }) => {
  await login(page); const soup = page.getByRole('article', { name: 'Блюдо: Суп дня' }); await expect(soup).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('dish-catalog-new-version'); await expect(soup).toContainText('Ингредиентов2'); await expect(soup).toContainText('Доступные порции7')
  const opener = page.getByRole('button', { name: 'Состав: Суп дня', exact: true })
  await opener.click(); await expect(page.getByRole('dialog')).toContainText('Сахар — 0,001 г на порцию'); await page.keyboard.press('Escape')
  for (const [scenario, title] of [['dish-detail-missing', 'Блюдо не найдено'], ['dish-detail-error', 'Не удалось загрузить состав']]) {
    await page.getByLabel('Сценарий API').selectOption(scenario); await opener.click(); await expect(page.getByRole('heading', { name: title })).toBeVisible(); await expect(page.getByText(/Молоко.*на порцию/)).toHaveCount(0); await page.keyboard.press('Escape'); await expect(opener).toBeFocused()
  }
  await page.getByLabel('Сценарий API').selectOption('dish-detail-slow'); await opener.click(); await expect(page.getByRole('status', { name: 'Загрузка состава' })).toBeVisible(); await expect(page.getByText(/Молоко.*на порцию/)).toHaveCount(0); await expect(page.getByRole('dialog')).toContainText('Молоко — 100 мл на порцию')
})
for (const role of ['manager', 'storekeeper', 'order_operator']) {
  test(`F19 denies ${role} before list or editor requests`, async ({ page }) => {
    await login(page, role, '/'); await expect(page.getByRole('heading', { name: 'Требуют внимания' })).toBeVisible()
    const reads: string[] = []; page.on('request', (request) => { if (/\/api\/v1\/dishes/.test(request.url())) reads.push(request.url()) })
    for (const path of ['/admin/dishes', '/admin/dishes/new', '/admin/dishes/00000000-0000-4000-8000-000000000800/edit', '/admin/recipes']) { await page.goto(path); await expect(page.getByRole('heading', { name: 'Недостаточно прав' })).toBeVisible(); await expect(page.getByRole('article')).toHaveCount(0) }
    expect(reads).toEqual([])
  })
}
