import { expect, test, type Page } from '@playwright/test'

async function login(page: Page, search = 'Молоко', role = 'manager') {
  await page.goto(`/inventory?search=${encodeURIComponent(search)}`)
  if (role !== 'manager') await page.getByLabel('Сценарий API').selectOption(`role-${role === 'order_operator' ? 'order-operator' : role}`)
  await page.getByLabel('Email', { exact: false }).fill('employee@example.test')
  await page.getByLabel(/^Пароль/).fill('local-test-only')
  await page.getByRole('button', { name: 'Войти', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Склад', exact: true })).toBeVisible()
}
async function open(page: Page, product = 'Молоко', line = 1) {
  await page.getByRole('button', { name: `Раскрыть партии ${product}`, exact: true }).click()
  const trigger = page.getByRole('button', { name: `Списать партию · строка ${line}`, exact: true })
  await trigger.focus(); await page.keyboard.press('Enter')
  await expect(page.getByRole('dialog', { name: 'Списать из партии', exact: true })).toBeVisible()
}
async function fill(page: Page, quantity = '0,001125', reason = 'Повреждена упаковка') {
  await page.getByLabel(/^Количество списания/).fill(quantity)
  await page.getByLabel(/^Причина списания/).fill(reason)
}
async function confirm(page: Page) {
  await page.getByRole('button', { name: 'Списать', exact: true }).click()
  await expect(page.getByRole('dialog', { name: 'Подтвердить списание?' })).toBeVisible()
  await page.getByRole('button', { name: 'Подтвердить списание', exact: true }).click()
}
for (const width of [360, 390, 1024, 1440]) {
  test(`F25 confirmed expired write-off, exact quantity, focus and responsive dialog at ${width}`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 1000 })
    const posts: { key: string | undefined; body: unknown }[] = [], errors: string[] = []
    page.on('pageerror', (error) => errors.push(error.message))
    page.on('request', (request) => { if (request.url().endsWith('/write-offs') && request.method() === 'POST') posts.push({ key: request.headers()['idempotency-key'], body: request.postDataJSON() }) })
    await login(page); await open(page)
    const dialog = page.getByRole('dialog', { name: 'Списать из партии', exact: true })
    await expect(dialog).toContainText('ТЕСТ-101 · строка 1'); await expect(dialog).toContainText('Просрочена')
    await page.getByLabel(/^Количество списания/).fill('0')
    await page.getByRole('button', { name: 'Списать', exact: true }).click()
    await expect(page.getByText('Количество должно быть больше нуля.')).toBeVisible()
    await fill(page)
    await page.getByRole('button', { name: 'Списать', exact: true }).click()
    expect(posts).toHaveLength(0)
    await page.keyboard.press('Escape')
    await expect(page.getByRole('dialog', { name: 'Подтвердить списание?' })).not.toBeVisible()
    await expect(page.getByRole('button', { name: 'Списать', exact: true })).toBeFocused()
    await dialog.screenshot({ path: testInfo.outputPath(`write-off-${width}.png`) })
    expect(await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true)
    await confirm(page)
    const result = page.getByRole('dialog', { name: 'Списание выполнено' })
    await expect(result).toBeVisible(); await expect(result).toContainText('198,875 мл')
    expect(posts).toHaveLength(1); expect(posts[0].body).toEqual({ batch_id: '00000000-0000-4000-8000-000000000201', quantity: '1.125', reason: 'Повреждена упаковка' })
    expect(posts[0].key).toMatch(/^[0-9a-f-]{36}$/)
    await page.getByRole('button', { name: 'Готово', exact: true }).click()
    await expect(page.getByRole('button', { name: 'Списать партию · строка 1', exact: true })).toBeFocused()
    const row = page.getByRole('region', { name: 'Партии: Молоко' }).getByRole('listitem').first()
    await expect(row).toContainText('0,198875 л')
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    expect(errors).toEqual([])
    await page.goto('/stock-movements?batch_id=00000000-0000-4000-8000-000000000201&type=manual_writeoff')
    await expect(page.getByText('Повреждена упаковка', { exact: true }).filter({ visible: true }).first()).toBeVisible()
    await page.goto('/analytics/write-offs?product_id=00000000-0000-4000-8000-000000000010&reason=manual_writeoff&from=2026-10-11T00%3A00%3A00Z')
    await expect(page.getByText('Групп продуктов и единиц: 1. Страница 1.')).toBeVisible()
    await expect(page.getByText('1,125 мл', { exact: true }).filter({ visible: true }).first()).toBeVisible()
  })
}
test('F25 lost response survives reload and exact replay produces one movement', async ({ page }) => {
  const posts: { key: string | undefined; body: unknown }[] = []
  page.on('request', (request) => { if (request.url().endsWith('/write-offs') && request.method() === 'POST') posts.push({ key: request.headers()['idempotency-key'], body: request.postDataJSON() }) })
  await login(page); await page.getByLabel('Сценарий API').selectOption('writeoff-lost-response'); await open(page); await fill(page); await confirm(page)
  await expect(page.getByText(/Результат отправки неизвестен/)).toBeVisible()
  await expect(page.getByLabel(/^Количество списания/)).toBeDisabled()
  await page.reload()
  await expect(page.getByRole('dialog', { name: 'Списать из партии', exact: true })).toBeVisible()
  await expect(page.getByLabel(/^Причина списания/)).toHaveValue('Повреждена упаковка')
  await page.getByRole('button', { name: 'Повторить исходный запрос' }).click()
  await expect(page.getByRole('dialog', { name: 'Списание выполнено' })).toBeVisible()
  expect(posts).toHaveLength(2); expect(posts[1]).toEqual(posts[0])
  const records = await page.evaluate(() => JSON.parse(sessionStorage.getItem('flagman.mock.write-offs.v1') ?? '[]'))
  expect(records.filter((item: { movement?: unknown }) => item.movement)).toHaveLength(1)
  await page.getByRole('link', { name: 'Открыть движение в истории' }).click()
  await expect(page.getByText('Повреждена упаковка', { exact: true }).filter({ visible: true }).first()).toBeVisible()
})
test('F25 competing order retains form and rereads the reduced physical stock before correction', async ({ page }) => {
  const keys: string[] = []
  page.on('request', (request) => { if (request.url().endsWith('/write-offs') && request.method() === 'POST') keys.push(request.headers()['idempotency-key']) })
  await login(page); await page.getByLabel('Сценарий API').selectOption('writeoff-stock-changed'); await open(page, 'Молоко', 2); await fill(page, '0.4'); await confirm(page)
  await expect(page.getByText(/Остаток партии изменился/)).toBeVisible()
  await expect(page.getByLabel(/^Количество списания/)).toHaveValue('0.4'); await expect(page.getByLabel(/^Причина списания/)).toHaveValue('Повреждена упаковка')
  await page.getByRole('button', { name: 'Перечитать остаток' }).click()
  await expect(page.getByText('Физический остаток: 0,25 л', { exact: true })).toBeVisible()
  await page.getByLabel(/^Количество списания/).fill('0.25'); await confirm(page)
  await expect(page.getByRole('dialog', { name: 'Списание выполнено' })).toBeVisible(); expect(keys[1]).not.toBe(keys[0])
  await expect(page.getByText(/Остаток после движения: 0 мл/)).toBeVisible()
  await page.getByRole('button', { name: 'Готово' }).click()
  await expect(page.getByRole('button', { name: 'Списать партию · строка 2', exact: true })).toHaveCount(0)
  await expect(page.getByLabel('Работа со складом')).toBeFocused()
})
test('F25 server field error focuses reason and Escape guards a dirty form', async ({ page }) => {
  await login(page); await page.getByLabel('Сценарий API').selectOption('writeoff-validation'); await open(page); await fill(page); await confirm(page)
  await expect(page.getByText('Сервер отклонил это значение. Проверьте поле.')).toBeVisible()
  await expect(page.getByLabel(/^Причина списания/)).toBeFocused()
  await page.keyboard.press('Escape'); await expect(page.getByRole('dialog', { name: 'Закрыть без сохранения?' })).toBeVisible()
  await page.getByRole('button', { name: 'Остаться' }).click()
  await expect(page.getByLabel(/^Причина списания/)).toHaveValue('Повреждена упаковка')
  await page.keyboard.press('Escape'); await page.getByRole('button', { name: 'Отбросить черновик' }).click()
  await expect(page.getByRole('button', { name: 'Списать партию · строка 1', exact: true })).toBeFocused()
})
test('F25 pending write blocks repeat and closing; does not optimistically change physical stock', async ({ page }) => {
  await login(page); await page.getByLabel('Сценарий API').selectOption('writeoff-slow'); await open(page); await fill(page); await confirm(page)
  await expect(page.getByRole('button', { name: 'Подождите…', exact: true })).toBeVisible()
  await expect(page.getByLabel(/^Количество списания/)).toBeDisabled(); await expect(page.getByRole('button', { name: 'Закрыть списание' })).toBeDisabled()
  await expect(page.getByText('Физический остаток: 0,2 л', { exact: true })).toBeVisible()
  await page.keyboard.press('Escape'); await expect(page.getByRole('dialog', { name: 'Списать из партии', exact: true })).toBeVisible()
  await expect(page.getByRole('dialog', { name: 'Списание выполнено' })).toBeVisible()
})
test('F25 integral pcs and depleted inactive product keep confirmed result visible', async ({ page }) => {
  await login(page, 'Яйцо'); await open(page, 'Яйцо', 27); await fill(page, '1.5')
  await page.getByRole('button', { name: 'Списать', exact: true }).click(); await expect(page.getByText('Количество штук должно быть целым.')).toBeVisible()
  await page.getByLabel(/^Количество списания/).fill('10'); await confirm(page); await expect(page.getByRole('dialog', { name: 'Списание выполнено' })).toBeVisible()
  await expect(page.getByText(/Остаток после движения: 0 шт/)).toBeVisible()
  await page.getByRole('button', { name: 'Готово' }).click()
  await page.goto('/inventory?search=Творог'); await page.getByRole('button', { name: /^С просрочкой/ }).click(); await open(page, 'Творог — только просрочка', 24); await fill(page, '0.3'); await confirm(page)
  await expect(page.getByRole('dialog', { name: 'Списание выполнено' })).toBeVisible()
  await expect(page.getByText(/Остаток после движения: 0 г/)).toBeVisible()
})
for (const role of ['admin', 'storekeeper', 'order_operator']) {
  test(`F25 role boundary ${role}`, async ({ page }) => {
    await login(page, 'Молоко', role)
    await page.getByRole('button', { name: 'Раскрыть партии Молоко' }).click()
    if (role === 'order_operator') { await expect(page.getByText('ТЕСТ-101 · строка 1', { exact: true })).toBeVisible(); await expect(page.getByRole('button', { name: /Списать партию/ })).toHaveCount(0) }
    else { await page.getByRole('button', { name: 'Списать партию · строка 1', exact: true }).click(); await fill(page); await confirm(page); await expect(page.getByRole('dialog', { name: 'Списание выполнено' })).toBeVisible() }
  })
}
