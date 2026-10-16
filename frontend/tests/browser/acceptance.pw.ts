import { expect, test, type Page } from '@playwright/test'

async function reception(page: Page) {
  await page.goto('/supplies/new')
  await page.getByLabel(/^Email/).fill('employee@example.test')
  await page.getByLabel(/^Пароль/).fill('local-test-only')
  await page.getByRole('button', { name: 'Войти', exact: true }).click()
  await page.getByLabel(/^Номер поставки/).fill('F31-НЕПОТЕРЯННЫЕ-ДАННЫЕ')
  await page.getByLabel(/^Дата приёмки/).fill('2098-01-01T12:00')
  const product = page.getByLabel('Продукт, строка 1', { exact: false })
  await expect(product).toContainText('Молоко')
  await product.selectOption({ label: 'Молоко' })
  await page.getByLabel('Количество, строка 1', { exact: false }).fill('0,5')
  await page.getByLabel('Годен до, строка 1', { exact: false }).fill('2099-01-01T12:00')
}
for (const [scenario, status, uncertain] of [
  ['reception-too-large', 413, false], ['reception-rate-limit', 429, true], ['reception-server-error', 500, true],
] as const) test(`F31 receipt ${status}: entered fields retained and recovery stays explicit`, async ({ page }) => {
  await reception(page)
  const attempts: { key: string | undefined; body: string | null }[] = []
  page.on('request', (request) => { if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/supplies') attempts.push({ key: request.headers()['idempotency-key'], body: request.postData() }) })
  await page.getByLabel('Сценарий API').selectOption(scenario)
  const response = page.waitForResponse((response) => new URL(response.url()).pathname === '/api/v1/supplies' && response.status() === status)
  await page.getByRole('button', { name: 'Принять поставку' }).click()
  await response
  const number = page.getByLabel(/^Номер поставки/)
  await expect(number).toHaveValue('F31-НЕПОТЕРЯННЫЕ-ДАННЫЕ')
  await expect(page.getByLabel('Количество, строка 1', { exact: false })).toHaveValue('0,5')
  if (uncertain) await expect(number).toBeDisabled()
  else await expect(number).toBeEnabled()
  await page.getByLabel('Сценарий API').selectOption('success')
  await page.getByRole('button', { name: uncertain ? 'Повторить исходный запрос' : 'Принять поставку', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Поставка принята' })).toBeVisible()
  expect(attempts).toHaveLength(2)
  expect(attempts[1].body).toBe(attempts[0].body)
  if (uncertain) expect(attempts[1].key).toBe(attempts[0].key)
  else expect(attempts[1].key).not.toBe(attempts[0].key)
})

for (const [target, scenario, label, ready] of [
  ['/inventory', 'slow', 'Загрузка склада', 'Молоко'],
  ['/supplies/new', 'products-slow', 'Загрузка продуктов', 'Новая поставка'],
] as const) test(`F31 initial loading without a cached snapshot: ${target}`, async ({ page }) => {
  await page.goto(`/login?returnTo=${encodeURIComponent(target)}`)
  await page.getByLabel('Сценарий API').selectOption(scenario)
  await page.getByLabel(/^Email/).fill('employee@example.test')
  await page.getByLabel(/^Пароль/).fill('local-test-only')
  await page.getByRole('button', { name: 'Войти', exact: true }).click()
  await expect(page.getByRole('status', { name: label })).toBeVisible()
  if (target === '/supplies/new') await page.getByLabel(/^Номер поставки/).fill('F31-INITIAL-DRAFT')
  await expect(page.getByRole('status', { name: label })).toHaveCount(0)
  await expect(page.getByRole('heading', { name: ready, exact: true })).toBeVisible()
  if (target === '/supplies/new') await expect(page.getByLabel(/^Номер поставки/)).toHaveValue('F31-INITIAL-DRAFT')
})

test('F31 recipe changes after a successful check: final refusal retains basket until explicit review', async ({ page }) => {
  await page.goto('/login?returnTo=%2Forders%2Fnew')
  await page.getByLabel(/^Email/).fill('employee@example.test')
  await page.getByLabel(/^Пароль/).fill('local-test-only')
  await page.getByRole('button', { name: 'Войти', exact: true }).click()
  await page.getByRole('button', { name: 'Добавить Суп дня', exact: true }).click()
  const submit = page.getByRole('button', { name: 'Оформить заказ', exact: true })
  await expect(submit).toBeEnabled()
  const posts: unknown[] = []
  page.on('request', (request) => {
    if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/orders') posts.push(request.postDataJSON())
  })
  await page.evaluate(async () => {
    const path = '/src/app/mocks/scenarios.ts'
    const { setMockScenario } = await import(path)
    setMockScenario('order-final-recipe')
  })
  const refused = page.waitForResponse((response) => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/orders' && response.status() === 409)
  await submit.click()
  expect((await (await refused).json()).error.code).toBe('RECIPE_CHANGED')
  const review = page.getByRole('region', { name: 'Изменение рецептуры' })
  await expect(review).toContainText('1 → 2')
  await expect(review).toContainText('150 мл')
  await expect(page.getByLabel(/^Порции: Суп дня/)).toHaveValue('1')
  await expect(submit).toBeDisabled()
  await page.getByLabel('Сценарий API').selectOption('order-recipe-changed')
  await page.getByRole('button', { name: 'Подтверждаю актуальный состав' }).click()
  await expect(submit).toBeEnabled()
  expect(posts).toEqual([{ items: [{ dish_id: '00000000-0000-4000-8000-000000000800', portions: 1, recipe_version: 1 }] }])
})
