import { test, expect, type Page } from '@playwright/test'
const milkId = '00000000-0000-4000-8000-000000000010', categoryId = '00000000-0000-4000-8000-000000000002'
async function login(page: Page, target: string) {
  await page.goto(`/login?returnTo=${encodeURIComponent(target)}`); await page.getByLabel('Сценарий API').selectOption('role-admin')
  await page.getByLabel(/^Email/).fill('employee@example.test'); await page.getByLabel(/^Пароль/).fill('local-test-only'); await page.getByRole('button', { name: 'Войти', exact: true }).click()
}
async function scenario(page: Page, value: string) {
  await page.evaluate((value) => { const select = document.querySelector<HTMLSelectElement>('#mock-scenario')!; select.value = value; select.dispatchEvent(new Event('change', { bubbles: true })) }, value)
}
const name = (page: Page) => page.getByLabel('Название продукта', { exact: true })
const save = (page: Page) => page.getByRole('button', { name: 'Сохранить продукт', exact: true })
for (const width of [360, 390, 1024, 1440]) test(`F17 full product, category preservation and keyboard save at ${width}px`, async ({ page }) => {
  const errors: string[] = [], writes: { path: string; body: unknown }[] = []
  page.on('pageerror', (error) => errors.push(error.message)); page.on('request', (request) => { if (request.method() === 'POST') writes.push({ path: new URL(request.url()).pathname, body: request.postDataJSON() }) })
  await page.setViewportSize({ width, height: 1000 }); await login(page, '/admin/products/new?returnTo=%2Fadmin%2Fproducts%3Fis_active%3Dfalse')
  await expect(page.getByLabel('Категория продукта')).toContainText('Бакалея'); await save(page).click(); await expect(name(page)).toBeFocused(); await expect(page.getByText(/Название: от 1 до 200/)).toBeVisible()
  await name(page).fill(`  Новый продукт 😀 ${width}  `); await page.getByLabel('Категория продукта').selectOption(categoryId)
  await page.getByLabel('Базовая единица').selectOption('pcs'); await expect(page.getByLabel('Единица отображения')).toHaveValue('pcs')
  await page.getByLabel('Целевой запас', { exact: true }).fill('1.001'); await save(page).click(); await expect(page.getByText('Количество штук должно быть целым.')).toBeVisible()
  await page.getByLabel('Целевой запас', { exact: true }).fill('0'); await page.getByLabel('Стандартный срок хранения, часы').fill('72'); await page.getByLabel('Продукт активен').uncheck()
  const opener = page.getByRole('button', { name: 'Создать категорию для продукта' }); await opener.focus(); await page.keyboard.press('Enter'); await page.getByLabel(/^Название категории/).fill(`Категория F17 ${width}`); await page.getByLabel(/^Название категории/).press('Enter')
  await expect(opener).toBeFocused(); await expect(page.getByLabel('Категория продукта').locator('option:checked')).toHaveText(`Категория F17 ${width}`); await expect(name(page)).toHaveValue(`  Новый продукт 😀 ${width}  `); await expect(page.getByLabel('Целевой запас', { exact: true })).toHaveValue('0'); await expect(page.getByLabel('Продукт активен')).not.toBeChecked()
  if (width >= 1024) { const base = await page.getByLabel('Базовая единица').boundingBox(), display = await page.getByLabel('Единица отображения').boundingBox(); expect(Math.abs(base!.y - display!.y)).toBeLessThan(1) }
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); await page.screenshot({ path: `test-results/f17-form-${width}.png`, fullPage: true })
  await save(page).focus(); await page.keyboard.press('Enter'); await expect(page.getByRole('heading', { name: `Продукт сохранён: Новый продукт 😀 ${width}` })).toBeVisible()
  const productWrites = writes.filter((write) => write.path === '/api/v1/products'); expect(productWrites).toHaveLength(1); expect(productWrites[0].body).toMatchObject({ name: `Новый продукт 😀 ${width}`, unit: 'pcs', display_unit: 'pcs', target_stock: '0.000', standard_shelf_life_hours: 72, is_active: false })
  await page.getByRole('link', { name: 'Вернуться в каталог', exact: true }).click(); await expect(page.getByRole('article', { name: `Продукт: Новый продукт 😀 ${width}` })).toContainText('Отключён'); expect(new URL(page.url()).searchParams.get('is_active')).toBe('false'); expect(errors).toEqual([])
})
test('F17 conflict, failed reload, all-field review and changed PATCH preserve input', async ({ page }) => {
  await login(page, `/admin/products/${milkId}/edit`); await expect(name(page)).toHaveValue('Молоко')
  await name(page).fill('Мой продукт'); await page.getByLabel('Целевой запас', { exact: true }).fill('9000.001'); await scenario(page, 'product-revision-conflict'); await save(page).click(); await expect(page.getByText(/Продукт изменён другим пользователем/)).toBeVisible(); await expect(save(page)).toBeDisabled()
  await scenario(page, 'products-error'); await page.getByRole('button', { name: 'Загрузить актуальный продукт' }).click(); await expect(page.getByText(/Не удалось загрузить актуальный продукт/)).toBeVisible(); await expect(name(page)).toHaveValue('Мой продукт')
  await scenario(page, 'success'); await page.getByRole('button', { name: 'Загрузить актуальный продукт' }).click(); await expect(page.getByRole('heading', { name: /Актуальный продукт: Молоко \(обновлено\). Ревизия: 2/ })).toBeVisible(); await expect(page.getByText('123.000', { exact: true })).toBeVisible(); await expect(page.getByLabel('Целевой запас', { exact: true })).toHaveValue('9000.001'); await expect(save(page)).toBeDisabled()
  const body = page.waitForRequest((request) => request.method() === 'PATCH'); await page.getByRole('button', { name: 'Подтвердить актуальную ревизию' }).click(); await save(page).click(); expect((await body).postDataJSON()).toEqual({ name: 'Мой продукт', target_stock: '9000.001', expected_revision: 2 }); await expect(page.getByRole('heading', { name: 'Продукт сохранён: Мой продукт' })).toBeVisible()
})
test('F17 UNIT_LOCKED, PRODUCT_IN_USE, duplicate, 422 and 404 retain all fields', async ({ page }) => {
  await login(page, `/admin/products/${milkId}/edit`); await expect(name(page)).toHaveValue('Молоко'); await name(page).fill('Мой черновик'); await page.getByLabel('Базовая единица').selectOption('g')
  await scenario(page, 'product-unit-locked'); await save(page).click(); await expect(page.getByText(/Базовую единицу нельзя изменить/)).toBeVisible(); await expect(name(page)).toHaveValue('Мой черновик')
  await page.getByLabel('Базовая единица').selectOption('ml'); await page.getByLabel('Продукт активен').uncheck(); await scenario(page, 'product-in-use'); await save(page).click(); await expect(page.getByText(/Продукт нельзя отключить/)).toBeVisible(); await expect(page.getByText('Демонстрационное активное блюдо')).toBeVisible()
  await scenario(page, 'success'); await name(page).fill('Сахар'); await save(page).click(); await expect(page.getByText(/Продукт с таким названием уже существует/)).toBeVisible(); await expect(name(page)).toHaveValue('Сахар')
  await name(page).fill('Мой черновик'); await scenario(page, 'product-validation-error'); await save(page).click(); await expect(page.getByText('Сервер: проверьте целевой запас')).toBeVisible(); await expect(page.getByLabel('Целевой запас', { exact: true })).toHaveAttribute('aria-invalid', 'true')
  await scenario(page, 'product-not-found'); await save(page).click(); await expect(page.getByText(/Продукт или категория не найдены/)).toBeVisible(); await expect(name(page)).toHaveValue('Мой черновик'); await expect(save(page)).toBeDisabled()
})
test('F17 reload restores full draft, departure stays/discards, logout erases owner data', async ({ page }) => {
  await login(page, '/admin/products/new'); await expect(page.getByLabel('Категория продукта')).toContainText('Бакалея'); await name(page).fill('Мой черновик'); await page.getByLabel('Категория продукта').selectOption(categoryId); await page.getByLabel('Целевой запас', { exact: true }).fill('1000.001'); await page.getByLabel('Стандартный срок хранения, часы').fill('96')
  await page.reload(); await expect(name(page)).toHaveValue('Мой черновик'); await expect(page.getByLabel('Целевой запас', { exact: true })).toHaveValue('1000.001'); await expect(page.getByLabel('Стандартный срок хранения, часы')).toHaveValue('96'); await expect(page.getByText(/Восстановлен черновик продукта/)).toBeVisible()
  await page.getByRole('link', { name: 'В каталог продуктов', exact: true }).click(); await page.getByRole('button', { name: 'Остаться', exact: true }).click(); await expect(name(page)).toHaveValue('Мой черновик')
  await page.getByRole('link', { name: 'В каталог продуктов', exact: true }).click(); await page.getByRole('button', { name: 'Уйти без сохранения', exact: true }).click(); await expect(page.getByRole('heading', { name: 'Каталог продуктов' })).toBeVisible(); expect(await page.evaluate(() => sessionStorage.getItem('flagman.products.v1'))).toBeNull()
  await page.getByRole('link', { name: 'Создать продукт', exact: true }).click(); await name(page).fill('Удалить при выходе'); await page.getByRole('button', { name: 'Выйти', exact: true }).click(); await page.getByRole('button', { name: 'Уйти без сохранения', exact: true }).click(); await expect(page.getByRole('heading', { name: 'Вход в Кладовую' })).toBeVisible(); expect(await page.evaluate(() => sessionStorage.getItem('flagman.products.v1'))).toBeNull()
})
test('F17 unknown POST is retained across reload and never automatically replayed', async ({ page }) => {
  let writes = 0; page.on('request', (request) => { if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/products') writes++ })
  await login(page, '/admin/products/new'); await expect(page.getByLabel('Категория продукта')).toContainText('Бакалея'); await name(page).fill('Потерянный ответ'); await page.getByLabel('Категория продукта').selectOption(categoryId); await scenario(page, 'product-lost-response'); await save(page).click(); await expect(page.getByText(/Результат сохранения неизвестен/)).toBeVisible(); await expect(save(page)).toBeDisabled(); expect(writes).toBe(1)
  await page.reload(); await expect(name(page)).toHaveValue('Потерянный ответ'); await expect(save(page)).toBeDisabled(); await name(page).press('Enter'); expect(writes).toBe(1); await expect(page.getByRole('button', { name: 'Я проверил каталог — разрешить новую отправку' })).toBeVisible()
})
test('F17 pending save blocks editing, repeat submit and departure; old batches stay identical', async ({ page }) => {
  await login(page, `/admin/products/${milkId}/edit`); await expect(name(page)).toHaveValue('Молоко')
  const batches = await page.evaluate(async (id) => (await fetch(`/api/v1/products/${id}/batches?status=all&sort=expires_at&page=1&page_size=50`)).json(), milkId)
  await page.getByLabel('Стандартный срок хранения, часы').fill('72'); await scenario(page, 'product-save-slow'); let writes = 0; page.on('request', (request) => { if (request.method() === 'PATCH') writes++ }); await save(page).click(); await expect(name(page)).toBeDisabled(); await page.getByRole('link', { name: 'В каталог продуктов', exact: true }).click(); await expect(page.getByRole('button', { name: 'Уйти без сохранения', exact: true })).toBeDisabled(); await page.getByRole('button', { name: 'Остаться', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Продукт сохранён: Молоко' })).toBeVisible(); expect(writes).toBe(1)
  const after = await page.evaluate(async (id) => (await fetch(`/api/v1/products/${id}/batches?status=all&sort=expires_at&page=1&page_size=50`)).json(), milkId); expect(after).toEqual(batches)
})
test('F17 fresh resource reads handle loading, failure, retry, missing and forbidden', async ({ page }) => {
  await login(page, '/admin/products'); await expect(page.getByRole('article', { name: 'Продукт: Молоко' })).toBeVisible(); await scenario(page, 'products-slow'); await page.getByRole('link', { name: 'Редактировать: Молоко' }).click(); await expect(page.getByRole('status', { name: 'Загрузка продукта для изменения' })).toBeVisible(); await expect(name(page)).toHaveValue('Молоко')
  await page.getByRole('link', { name: 'В каталог продуктов', exact: true }).click(); await expect(page.getByRole('article', { name: 'Продукт: Молоко' })).toBeVisible(); await scenario(page, 'products-error'); await page.getByRole('link', { name: 'Редактировать: Молоко' }).click(); await expect(page.getByRole('heading', { name: 'Не удалось прочитать продукт' })).toBeVisible(); await expect(name(page)).toHaveCount(0)
  await scenario(page, 'success'); await page.getByRole('button', { name: 'Попробовать снова' }).click(); await expect(name(page)).toHaveValue('Молоко'); await page.goto('/admin/products/00000000-0000-4000-8000-ffffffffffff/edit'); await expect(page.getByRole('heading', { name: 'Продукт не найден' })).toBeVisible()
  await scenario(page, 'forbidden')
  // Direct SPA address entry keeps the selected MSW scenario (document reload resets it).
  await page.evaluate((id) => {
    const state = { ...history.state, idx: history.state.idx + 1 }
    history.pushState(state, '', `/admin/products/${id}/edit`)
    window.dispatchEvent(new PopStateEvent('popstate', { state }))
  }, milkId)
  await expect(page.getByRole('heading', { name: 'Недостаточно прав' })).toBeVisible(); await expect(name(page)).toHaveCount(0)


})


test('F17 maximum Unicode category remains selectable and product draft can be saved', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 1000 }); await login(page, '/admin/products/new'); await expect(page.getByLabel('Категория продукта')).toContainText('Бакалея'); const category = '😀'.repeat(200)
  await name(page).fill('Юникодный продукт'); await page.getByRole('button', { name: 'Создать категорию для продукта' }).click(); await page.getByLabel(/^Название категории/).fill(category); await page.getByLabel(/^Название категории/).press('Enter')
  await expect(page.getByLabel('Категория продукта').locator('option:checked')).toHaveText(category); await expect(save(page)).toBeEnabled(); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await save(page).click(); await expect(page.getByRole('heading', { name: 'Продукт сохранён: Юникодный продукт' })).toBeVisible()
})
