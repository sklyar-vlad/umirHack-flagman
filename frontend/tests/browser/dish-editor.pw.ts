import { backgroundRefresh } from './refresh'
import { test, expect, type Page } from '@playwright/test'
const soupId = '00000000-0000-4000-8000-000000000800', milk = '00000000-0000-4000-8000-000000000010', sugar = '00000000-0000-4000-8000-000000000012', eggs = '00000000-0000-4000-8000-000000000014'
async function login(page: Page, path = '/admin/dishes/new') {
  await page.goto(`/login?returnTo=${encodeURIComponent(path)}`); await page.getByLabel('Сценарий API').selectOption('role-admin')
  await page.getByLabel(/^Email/).fill('employee@example.test'); await page.getByLabel(/^Пароль/).fill('local-test-only'); await page.getByRole('button', { name: 'Войти', exact: true }).click()
}
const preview = (page: Page) => page.getByRole('button', { name: 'Проверить без сохранения', exact: true })
const save = (page: Page, edit = false) => page.getByRole('button', { name: edit ? 'Сохранить изменения блюда' : 'Создать блюдо', exact: true })
async function scenario(page: Page, value: string) {
  const refreshedProducts = page.waitForResponse((response) => new URL(response.url()).pathname === '/api/v1/products' && response.ok())
  await page.getByLabel('Сценарий API').selectOption(value)
  await refreshedProducts
  await expect(page.getByLabel(/^Продукт 1(?:$|\s)/)).toBeEnabled()
}
async function recipe(page: Page, product = milk, quantity = '100') { await expect(page.getByLabel(/^Продукт 1(?:$|\s)/)).toContainText('Молоко'); await page.getByLabel(/^Продукт 1(?:$|\s)/).selectOption(product); await page.getByLabel(/^Расход 1(?:$|\s)/).fill(quantity) }
for (const width of [360, 390, 1024, 1440]) {
  test(`F20 creation, exact validation, preview and keyboard ${width}px`, async ({ page }) => {
    const writes: { path: string; body: unknown; key: string | null }[] = [], errors: string[] = []
    page.on('pageerror', (error) => errors.push(error.message)); page.on('request', (request) => { if (request.method() === 'POST' && request.url().includes('/api/v1/dishes')) writes.push({ path: new URL(request.url()).pathname, body: request.postDataJSON(), key: request.headers()['idempotency-key'] ?? null }) })
    await page.setViewportSize({ width, height: 1000 }); await login(page); await recipe(page)
    // Preview needs a valid recipe but does not require a name or save a record.
    await preview(page).focus(); await page.keyboard.press('Enter'); await expect(page.getByRole('region', { name: 'Проверка рецептуры' })).toContainText('Доступные порции: 15')
    await page.getByLabel(/^Название блюда(?:$|\s)/).fill(`Новое блюдо ${width}`); await expect(page.getByRole('region', { name: 'Проверка рецептуры' })).toHaveCount(0)
    await page.getByLabel('Описание блюда').fill('Описание тестовой рецептуры'); await page.getByRole('button', { name: 'Добавить ингредиент' }).click(); await page.getByLabel(/^Продукт 2(?:$|\s)/).selectOption(sugar); await page.getByLabel(/^Расход 2(?:$|\s)/).fill('0,001')
    expect(await page.getByLabel(/^Продукт 2(?:$|\s)/).locator(`option[value="${milk}"]`).isDisabled()).toBe(true)
    await preview(page).click(); const result = page.getByRole('region', { name: 'Проверка рецептуры' }); await expect(result).toContainText('Есть дефицит'); await expect(result).toContainText('не хватает 0,001 г')
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); await page.screenshot({ path: `test-results/f20-editor-${width}.png`, fullPage: true })
    await page.getByLabel(/^Продукт 2(?:$|\s)/).selectOption(eggs); await page.getByLabel(/^Расход 2(?:$|\s)/).fill('1.5'); await save(page).click(); await expect(page.getByText('Количество штук должно быть целым.', { exact: true })).toBeVisible(); expect(writes.filter((entry) => entry.path === '/api/v1/dishes')).toHaveLength(0)
    await page.getByLabel(/^Расход 2(?:$|\s)/).fill('2'); await save(page).focus(); await page.keyboard.press('Enter'); await expect(page.getByRole('heading', { name: `Блюдо сохранено: Новое блюдо ${width}` })).toBeVisible()
    expect(writes.filter((entry) => entry.path === '/api/v1/dishes')).toEqual([{ path: '/api/v1/dishes', body: { name: `Новое блюдо ${width}`, description: 'Описание тестовой рецептуры', is_active: true, recipe: [{ product_id: milk, quantity_per_portion: '100.000' }, { product_id: eggs, quantity_per_portion: '2.000' }] }, key: null }])
    expect(await page.evaluate(() => sessionStorage.getItem('flagman.dishes.v1'))).toBeNull(); await page.getByRole('link', { name: 'Вернуться в каталог' }).click(); await expect(page.getByRole('article', { name: `Блюдо: Новое блюдо ${width}` })).toContainText('Ингредиентов2'); expect(errors).toEqual([])
  })
}
test('F20 late preview, error/retry and metadata invalidate previous result', async ({ page }) => {
  await login(page); await recipe(page); await scenario(page, 'dish-preview-slow'); await preview(page).click(); await expect(page.getByRole('button', { name: 'Проверяем рецептуру…' })).toBeVisible(); await page.getByLabel(/^Расход 1(?:$|\s)/).fill('200')
  await page.waitForTimeout(1700); await expect(page.getByRole('region', { name: 'Проверка рецептуры' })).toHaveCount(0); await expect(page.getByText(/Предыдущая проверка неактуальна/)).toBeVisible()
  await scenario(page, 'dish-preview-error'); await preview(page).click(); await expect(page.getByRole('heading', { name: 'Не удалось проверить рецептуру' })).toBeVisible(); await scenario(page, 'success'); await page.getByRole('button', { name: 'Попробовать снова' }).click(); await expect(page.getByRole('region', { name: 'Проверка рецептуры' })).toContainText('Доступные порции: 7')
  await page.getByLabel('Описание блюда').fill('Изменено'); await expect(page.getByRole('region', { name: 'Проверка рецептуры' })).toHaveCount(0)
})
test('F20 full replacement, metadata omission, fresh recipe version and history unchanged', async ({ page }) => {
  const patches: unknown[] = []
  page.on('request', (request) => { if (request.method() === 'PATCH' && request.url().includes('/dishes/')) patches.push(request.postDataJSON()) })
  await login(page, `/admin/dishes/${soupId}/edit`); await expect(page.getByLabel(/^Название блюда(?:$|\s)/)).toHaveValue('Суп дня')
  const original = await page.evaluate(async () => (await fetch('/api/v1/orders?page=1&page_size=50&sort=-created_at')).json())
  await page.getByLabel('Описание блюда').fill(''); await save(page, true).click(); await expect(page.getByRole('region', { name: 'Результат сохранения блюда' })).toContainText('Версия рецептуры: 1'); expect(patches[0]).toEqual({ expected_revision: 1, description: null })
  await page.getByRole('link', { name: 'Вернуться в каталог' }).click(); await page.getByRole('link', { name: 'Редактировать: Суп дня' }).click(); await expect(page.getByLabel('Описание блюда')).toHaveValue(''); await page.getByRole('button', { name: 'Добавить ингредиент' }).click(); await page.getByLabel(/^Продукт 2(?:$|\s)/).selectOption(eggs); await page.getByLabel(/^Расход 2(?:$|\s)/).fill('1'); await save(page, true).click()
  await expect(page.getByRole('region', { name: 'Результат сохранения блюда' })).toContainText('Версия рецептуры: 2'); expect(patches[1]).toEqual({ expected_revision: 2, recipe: [{ product_id: milk, quantity_per_portion: '100.000' }, { product_id: eggs, quantity_per_portion: '1.000' }] })
  const after = await page.evaluate(async () => (await fetch('/api/v1/orders?page=1&page_size=50&sort=-created_at')).json()); expect(after).toEqual(original)
  await page.getByRole('link', { name: 'Вернуться в каталог' }).click(); await page.getByRole('button', { name: 'Состав: Суп дня' }).click(); await expect(page.getByRole('dialog')).toContainText('Яйцо — 1 шт. на порцию')
})
test('F20 revision conflict retains input, reload and explicit review precede PATCH', async ({ page }) => {
  await login(page, `/admin/dishes/${soupId}/edit`); await expect(page.getByLabel(/^Название блюда(?:$|\s)/)).toHaveValue('Суп дня'); await page.getByLabel(/^Название блюда(?:$|\s)/).fill('Моё имя'); await scenario(page, 'dish-revision-conflict'); await save(page, true).click(); await expect(page.getByText(/Блюдо изменено другим пользователем/)).toBeVisible(); await expect(save(page, true)).toBeDisabled()
  await page.getByRole('button', { name: 'Загрузить актуальную версию блюда' }).click(); await expect(page.getByRole('region', { name: 'Проверка актуальной версии' })).toContainText('Суп дня (обновлено)'); await expect(page.getByLabel(/^Название блюда(?:$|\s)/)).toHaveValue('Моё имя'); await expect(save(page, true)).toBeDisabled(); await page.getByRole('button', { name: 'Подтверждаю проверку всех полей блюда' }).click(); await save(page, true).click(); await expect(page.getByRole('heading', { name: 'Блюдо сохранено: Моё имя' })).toBeVisible(); await expect(page.getByRole('region', { name: 'Результат сохранения блюда' })).toContainText('Ревизия: 3')
})
test('F20 422/duplicate retain and focus field; owner draft survives reload', async ({ page }) => {
  await login(page); await recipe(page); await page.getByLabel(/^Название блюда(?:$|\s)/).fill('Черновик')
  await scenario(page, 'dish-validation'); await save(page).click(); await expect(page.getByText('Сервер: проверьте расход', { exact: true })).toBeVisible(); await expect(page.getByLabel(/^Расход 1(?:$|\s)/)).toBeFocused()
  await scenario(page, 'dish-duplicate'); await save(page).click(); await expect(page.getByText('Сервер: повтор ингредиента', { exact: true })).toBeVisible(); await expect(page.getByLabel(/^Продукт 1(?:$|\s)/)).toBeFocused()
  await page.reload(); await expect(page.getByLabel(/^Название блюда(?:$|\s)/)).toHaveValue('Черновик'); await expect(page.getByLabel(/^Расход 1(?:$|\s)/)).toHaveValue('100'); await expect(page.getByText(/Восстановлен черновик блюда/)).toBeVisible()
  await page.getByRole('link', { name: 'В каталог блюд', exact: true }).click(); await page.getByRole('button', { name: 'Остаться', exact: true }).click(); await expect(page.getByLabel(/^Название блюда(?:$|\s)/)).toHaveValue('Черновик')
})
test('F20 lost response never automatically repeats after reload', async ({ page }) => {
  let writes = 0; page.on('request', (request) => { if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/dishes') writes++ })
  await login(page); await recipe(page); await page.getByLabel(/^Название блюда(?:$|\s)/).fill('Неизвестный исход'); await scenario(page, 'dish-lost-response'); await save(page).click(); await expect(page.getByText(/Результат сохранения неизвестен/)).toBeVisible(); await expect(save(page)).toBeDisabled(); await page.reload(); await expect(page.getByLabel(/^Название блюда(?:$|\s)/)).toHaveValue('Неизвестный исход'); await expect(save(page)).toBeDisabled(); expect(writes).toBe(1)
})
test('F20 saving blocks editing, double submission and dirty departure', async ({ page }) => {
  let writes = 0; page.on('request', (request) => { if (request.method() === 'PATCH') writes++ })
  await login(page, `/admin/dishes/${soupId}/edit`); await expect(page.getByLabel(/^Название блюда(?:$|\s)/)).toHaveValue('Суп дня'); await page.getByLabel(/^Расход 1(?:$|\s)/).fill('120'); await scenario(page, 'dish-save-slow'); await save(page, true).click(); await expect(page.getByLabel(/^Название блюда(?:$|\s)/)).toBeDisabled(); await page.getByRole('link', { name: 'В каталог блюд', exact: true }).click(); await expect(page.getByRole('button', { name: 'Уйти без сохранения', exact: true })).toBeDisabled(); await page.getByRole('button', { name: 'Остаться', exact: true }).click(); await expect(page.getByRole('heading', { name: 'Блюдо сохранено: Суп дня' })).toBeVisible(); expect(writes).toBe(1)
})
test('F20 active product options handle empty/error/loading/retry and resource denial', async ({ page }) => {
  await login(page); await expect(page.getByLabel(/^Продукт 1/)).toContainText('Молоко'); await scenario(page, 'products-empty'); await expect(page.getByRole('heading', { name: 'Активных продуктов пока нет' })).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('products-error'); await expect(page.getByRole('heading', { name: 'Не удалось загрузить активные продукты' })).toBeVisible(); await expect(preview(page)).toBeDisabled()
  await page.evaluate(async () => { const path = '/src/app/mocks/scenarios.ts'; const { setMockScenario } = await import(path); setMockScenario('success') }); await page.getByRole('button', { name: 'Попробовать снова' }).click(); await expect(page.getByLabel(/^Продукт 1/)).toContainText('Молоко')
  await backgroundRefresh(page, 'products-slow', '/api/v1/products', 'Загрузка активных продуктов', page.getByLabel(/^Продукт 1/))
  await page.getByLabel('Сценарий API').selectOption('success'); await page.getByRole('link', { name: 'В каталог блюд', exact: true }).click(); await page.getByLabel('Сценарий API').selectOption('dish-detail-slow'); await page.getByRole('link', { name: 'Редактировать: Суп дня' }).click(); await expect(page.getByRole('status', { name: 'Загрузка блюда для изменения' })).toBeVisible(); await expect(page.getByLabel(/^Название блюда/)).toHaveValue('Суп дня')
  await page.getByRole('link', { name: 'В каталог блюд', exact: true }).click(); await page.getByLabel('Сценарий API').selectOption('dish-detail-error'); await page.getByRole('link', { name: 'Редактировать: Суп дня' }).click(); await expect(page.getByRole('heading', { name: 'Не удалось прочитать блюдо' })).toBeVisible(); await expect(page.getByLabel(/^Название блюда/)).toHaveCount(0)
  await page.getByLabel('Сценарий API').selectOption('success'); await page.getByRole('button', { name: 'Попробовать снова' }).click(); await expect(page.getByLabel(/^Название блюда/)).toHaveValue('Суп дня')
  await page.getByRole('link', { name: 'В каталог блюд', exact: true }).click(); await page.getByLabel('Сценарий API').selectOption('dish-detail-missing'); await page.getByRole('link', { name: 'Редактировать: Суп дня' }).click(); await expect(page.getByRole('heading', { name: 'Блюдо не найдено' })).toBeVisible(); await expect(page.getByLabel(/^Название блюда/)).toHaveCount(0)
})
test('F20 saved unavailable dish updates stop-list and checked basket rechecks in the same store', async ({ page }) => {
  await login(page); await recipe(page, sugar, '1'); await page.getByLabel(/^Название блюда/).fill('Сахарное блюдо'); await save(page).click(); await expect(page.getByRole('heading', { name: 'Блюдо сохранено: Сахарное блюдо' })).toBeVisible(); await page.goto('/stop-list'); await expect(page.getByRole('article', { name: 'Блюдо: Сахарное блюдо' })).toBeVisible()
  await page.goto('/orders/new'); await page.getByRole('button', { name: 'Добавить Суп дня' }).click(); await expect(page.getByRole('button', { name: 'Оформить заказ', exact: true })).toBeEnabled()
  let checks = 0; page.on('request', (request) => { if (new URL(request.url()).pathname === '/api/v1/orders/check') checks++ })
  await page.evaluate(async (id) => {
    const storePath = '/src/app/store/store.ts', apiPath = '/src/entities/dish/index.ts'
    const { store } = await import(storePath), { dishApi } = await import(apiPath)
    await store.dispatch(dishApi.endpoints.updateDish.initiate({ id, body: { expected_revision: 1, description: 'Обновлённое описание' }, previousVersion: 1 })).unwrap()
  }, soupId)
  await expect(page.getByRole('button', { name: 'Оформить заказ', exact: true })).toBeEnabled(); await expect.poll(() => checks).toBeGreaterThan(0)
})
