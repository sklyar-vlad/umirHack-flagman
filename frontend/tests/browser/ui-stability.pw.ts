import { expect, test } from '@playwright/test'

for (const width of [360, 390, 1024, 1440]) test(`UI layout, focus, refresh and calendar at ${width}px`, async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  await page.setViewportSize({ width, height: 1000 })
  await page.goto('/login?returnTo=/admin/users')
  await page.getByLabel('Сценарий API').selectOption('role-admin')
  await page.getByLabel(/^Email/).fill('employee@example.test')
  await page.getByLabel(/^Пароль/).fill('local-test-only')
  await page.getByRole('button', { name: 'Войти', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Пользователи и роли' })).toBeVisible()
  await expect(page.getByRole('article', { name: 'Пользователь: Анна Администратор' })).toBeVisible()
  const search = page.getByLabel('Поиск по имени или email'), submit = page.getByRole('search').getByRole('button', { name: 'Найти', exact: true })
  const inputBox = (await search.boundingBox())!, submitBox = (await submit.boundingBox())!
  if (width >= 768) expect(Math.abs(inputBox.y + inputBox.height - submitBox.y - submitBox.height)).toBeLessThan(3)
  await search.fill('Незавершённый поиск'); await search.focus()
  let reads = 0
  page.on('request', (request) => { if (new URL(request.url()).pathname === '/api/v1/users') reads++ })
  await page.evaluate(() => { window.dispatchEvent(new Event('focus')); document.dispatchEvent(new Event('visibilitychange')) })
  await page.waitForTimeout(500)
  await expect(search).toBeFocused(); expect(reads).toBe(0)
  await page.evaluate(() => {
    const select = document.querySelector<HTMLSelectElement>('#mock-scenario')!
    select.value = 'users-slow'; select.dispatchEvent(new Event('change', { bubbles: true }))
  })
  await expect.poll(() => reads).toBeGreaterThan(0)
  await expect(page.getByRole('status', { name: 'Загрузка пользователей' })).toHaveCount(0)
  await expect(search).toBeFocused(); await expect(search).toHaveValue('Незавершённый поиск')
  await expect(page.getByRole('article', { name: 'Пользователь: Анна Администратор' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: `test-results/ui-users-${width}.png`, fullPage: true })

  await page.goto('/analytics/stop-list-forecast')
  await expect(page.getByRole('heading', { name: 'Прогноз стоп-листа', exact: true })).toBeVisible()
  if (width < 768) {
    const burger = page.getByRole('button', { name: 'Открыть меню' }), box = (await burger.boundingBox())!
    expect(box.x).toBeGreaterThanOrEqual(0); expect(box.x + box.width).toBeLessThanOrEqual(width)
    expect(box.width).toBe(44)
    await burger.click()
  }
  const nav = page.getByRole('navigation', { name: 'Разделы приложения' }).filter({ visible: true })
  await expect(nav.locator('[aria-current="page"]')).toHaveCount(1)
  await expect(nav.getByRole('link', { name: 'Прогноз', exact: true })).toHaveAttribute('aria-current', 'page')
  if (width < 768) { await page.keyboard.press('Escape'); await expect(page.getByRole('button', { name: 'Открыть меню' })).toBeFocused() }

  await page.goto('/supplies/new')
  await expect(page.getByLabel(/^Дата приёмки/)).toBeVisible()
  const received = page.getByLabel(/^Дата приёмки/)
  await received.fill('2030-01-03T12:30')
  const calendar = page.getByRole('button', { name: 'Выбрать дату и время' }).first()
  await calendar.click()
  const dialog = page.getByRole('dialog', { name: 'Выберите дату и время' })
  await expect(dialog).toBeVisible(); await expect(dialog).toContainText('январь 2030')
  await dialog.locator('[data-date="2030-01-04"]').click()
  await dialog.getByLabel('Время', { exact: true }).fill('16:45')
  await page.screenshot({ path: `test-results/ui-calendar-${width}.png`, fullPage: true })
  await dialog.getByRole('button', { name: 'Применить' }).click()
  await expect(received).toHaveValue('2030-01-04T16:45')
  await expect(calendar).toBeFocused()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  expect(errors).toEqual([])
})
