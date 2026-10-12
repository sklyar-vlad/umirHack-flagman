import { test, expect, type Page } from '@playwright/test'
const anna = 'Анна Администратор', boris = 'Борис Кладовщик', id = '00000000-0000-4000-8000-000000000901'
async function login(page: Page, role = 'admin', target = '/admin/users') {
  await page.goto(`/login?returnTo=${encodeURIComponent(target)}`)
  if (role !== 'manager') await page.getByLabel('Сценарий API').selectOption(`role-${role.replaceAll('_', '-')}`)
  await page.getByLabel(/^Email/).fill('employee@example.test'); await page.getByLabel(/^Пароль/).fill('local-test-only'); await page.getByRole('button', { name: 'Войти', exact: true }).click()
}
const card = (page: Page, name = anna) => page.getByRole('article', { name: `Пользователь: ${name}` })
for (const width of [360, 390, 1024, 1440]) {
  test(`F21 safe user list, role directory and keyboard entries at ${width}px`, async ({ page }) => {
    const errors: string[] = [], reads: URL[] = [], writes: string[] = []
    page.on('pageerror', (error) => errors.push(error.message))
    page.on('request', (request) => { const url = new URL(request.url()); if (/\/api\/v1\/(users|roles)/.test(url.pathname)) { reads.push(url); if (request.method() !== 'GET') writes.push(request.url()) } })
    await page.setViewportSize({ width, height: 1000 }); await login(page)
    await expect(card(page)).toContainText('anna@example.test'); await expect(card(page)).toContainText('Администратор'); await expect(card(page)).toContainText('12.10.2026, 09:15:00')
    await expect(card(page, boris)).toContainText('Отключён'); await expect(card(page, boris)).toContainText('Ещё не входил')
    await expect(page.getByLabel('Роль', { exact: true })).toBeEnabled()
    expect(Object.fromEntries(reads.find((url) => url.pathname === '/api/v1/users')!.searchParams)).toEqual({ role: 'all', is_active: 'all', sort: 'name', page: '1', page_size: '50' })
    expect(await page.getByLabel('Роль', { exact: true }).locator('option').allTextContents()).toEqual(['Все роли', 'Администратор', 'Менеджер', 'Кладовщик', 'Оператор заказов'])
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: `test-results/f21-users-${width}.png`, fullPage: false }); await card(page).screenshot({ path: `test-results/f21-user-card-${width}.png` })
    for (const [link, title, path] of [[`Редактировать: ${anna}`, 'Редактирование пользователя', `/admin/users/${id}/edit`], [`Сбросить пароль: ${anna}`, 'Сброс пароля пользователя', `/admin/users/${id}/password`], ['Создать пользователя', 'Создание пользователя', '/admin/users/new']]) {
      await page.getByRole('link', { name: link, exact: true }).focus(); await page.keyboard.press('Enter'); await expect(page.getByRole('heading', { name: title })).toBeVisible(); expect(new URL(page.url()).pathname).toBe(path)
      await expect(page.getByRole('heading', { name: 'Форма будет доступна в следующей истории' })).toBeVisible(); await page.getByRole('link', { name: 'В список пользователей' }).click(); await expect(card(page)).toBeVisible()
    }
    expect(reads.every((url) => !url.pathname.includes('/password') && !url.searchParams.has('password'))).toBe(true); expect(writes).toEqual([]); expect(errors).toEqual([])
  })
}
test('F21 URL pagination, AND filters, name/email literal search and back/forward', async ({ page }) => {
  await login(page); await expect(page.getByRole('article')).toHaveCount(25)
  await page.getByLabel('Поиск по имени или email').fill('pending@example.test')
  await page.getByRole('button', { name: 'Сбросить фильтры' }).first().click()
  await expect(page.getByLabel('Поиск по имени или email')).toHaveValue('')
  expect(new URL(page.url()).search).toBe('')
  await page.getByLabel('Пользователей на странице').selectOption('20'); await expect(page.getByRole('article')).toHaveCount(20)
  await page.getByRole('button', { name: 'Страница 2', exact: true }).click(); await expect(page.getByRole('article')).toHaveCount(5)
  await page.getByLabel('Роль', { exact: true }).selectOption('storekeeper'); await expect(card(page, boris)).toBeVisible(); expect(new URL(page.url()).searchParams.has('page')).toBe(false)
  await page.getByLabel('Активность', { exact: true }).selectOption('true'); await expect(page.getByRole('heading', { name: 'Ничего не найдено' })).toBeVisible(); await expect(page.getByRole('article')).toHaveCount(0)
  await page.goBack(); await expect(card(page, boris)).toBeVisible(); await page.goForward(); await expect(page.getByRole('heading', { name: 'Ничего не найдено' })).toBeVisible()
  await page.getByRole('button', { name: 'Сбросить фильтры' }).first().click(); await expect(card(page)).toBeVisible()
  await page.getByLabel('Поиск по имени или email').fill('VERA@EXAMPLE.TEST'); await page.keyboard.press('Enter'); await expect(card(page, 'Вера Оператор')).toBeVisible(); await expect(page.getByRole('article')).toHaveCount(1)
  await page.getByLabel('Поиск по имени или email').fill('%_\\'); await page.keyboard.press('Enter'); await expect(card(page, 'Менеджер 01 %_\\')).toBeVisible(); expect(new URL(page.url()).searchParams.get('search')).toBe('%_\\')
  await page.getByRole('link', { name: 'Редактировать: Менеджер 01 %_\\' }).click(); await page.getByRole('link', { name: 'В список пользователей' }).click(); expect(new URL(page.url()).searchParams.get('search')).toBe('%_\\')
  await page.goto('/admin/users?page=99&page_size=20'); await expect(page.getByRole('heading', { name: 'На этой странице нет пользователей' })).toBeVisible(); await page.getByRole('button', { name: 'К первой странице' }).click(); await expect(page.getByRole('article')).toHaveCount(20)
})
test('F21 loading, stale retry, initial error, empty and parameter isolation', async ({ page }) => {
  await login(page); await expect(card(page)).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('users-slow'); await expect(page.getByRole('status', { name: 'Загрузка пользователей' })).toBeVisible(); await expect(page.getByRole('button', { name: 'Обновить пользователей' })).toBeDisabled(); await expect(page.getByRole('status', { name: 'Загрузка пользователей' })).toHaveCount(0)
  await page.getByLabel('Сценарий API').selectOption('users-error'); await expect(page.getByText(/Данные могут быть устаревшими/)).toBeVisible(); await expect(page.getByRole('article')).toHaveCount(25)
  await page.evaluate(async () => { const path = '/src/app/mocks/scenarios.ts'; const { setMockScenario } = await import(path); setMockScenario('success') })
  await page.getByRole('button', { name: 'Попробовать снова' }).focus(); await page.keyboard.press('Enter'); await expect(page.getByRole('heading', { name: 'Не удалось загрузить пользователей' })).toHaveCount(0)
  await page.getByLabel('Сценарий API').selectOption('users-empty'); await expect(page.getByRole('heading', { name: 'Список пользователей пуст' })).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('users-error'); await page.getByLabel('Активность', { exact: true }).selectOption('false'); await expect(page.getByRole('heading', { name: 'Не удалось загрузить пользователей' })).toBeVisible(); await expect(page.getByRole('article')).toHaveCount(0)
  await page.getByLabel('Сценарий API').selectOption('success'); await expect(card(page, boris)).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('forbidden'); await expect(page.getByRole('heading', { name: 'Недостаточно прав' })).toBeVisible(); await expect(page.getByRole('article')).toHaveCount(0); await expect(page.getByText('boris@example.test')).toHaveCount(0)
})
test('F21 directory loading/error, independent retry and directory 403 hides cached users', async ({ page }) => {
  await login(page); await expect(card(page)).toBeVisible()
  await page.getByLabel('Сценарий API').selectOption('roles-slow'); await expect(page.getByRole('status', { name: 'Загрузка справочника ролей' })).toBeVisible(); await expect(page.getByRole('status', { name: 'Загрузка справочника ролей' })).toHaveCount(0)
  await page.getByLabel('Сценарий API').selectOption('roles-error'); await expect(page.getByRole('heading', { name: 'Не удалось загрузить роли' })).toBeVisible(); await expect(page.getByLabel('Роль', { exact: true })).toBeDisabled(); await expect(card(page)).toContainText('Администратор')
  await page.evaluate(async () => { const path = '/src/app/mocks/scenarios.ts'; const { setMockScenario } = await import(path); setMockScenario('success') })
  await page.getByRole('button', { name: 'Попробовать снова' }).click(); await expect(page.getByLabel('Роль', { exact: true })).toBeEnabled()
  await page.getByLabel('Сценарий API').selectOption('roles-forbidden'); await expect(page.getByRole('heading', { name: 'Недостаточно прав' })).toBeVisible(); await expect(page.getByRole('article')).toHaveCount(0)
})
test('F21 session restoration preserves only user filters and nested return URL', async ({ page, context }) => {
  await login(page, 'admin', '/admin/users?role=admin&page_size=20&password=unused#hash'); await expect(card(page)).toBeVisible()
  const target = `/admin/users/${id}/password?${new URLSearchParams({ returnTo: '/admin/users?role=admin&page_size=20&token=unused', password: 'unused' })}`
  await page.goto(target); await expect(page.getByRole('heading', { name: 'Сброс пароля пользователя' })).toBeVisible()
  await context.clearCookies(); await page.goto(target); await expect(page.getByRole('heading', { name: 'Вход в Кладовую' })).toBeVisible()
  const safe = new URL(page.url()).searchParams.get('returnTo')!; expect(safe).not.toContain('unused'); expect(new URL(safe, 'http://local').searchParams.get('returnTo')).toBe('/admin/users?role=admin&page_size=20')
  await page.getByLabel('Сценарий API').selectOption('role-admin'); await page.getByLabel(/^Email/).fill('employee@example.test'); await page.getByLabel(/^Пароль/).fill('local-test-only'); await page.getByRole('button', { name: 'Войти', exact: true }).click(); await expect(page.getByRole('heading', { name: 'Сброс пароля пользователя' })).toBeVisible()
  await page.getByRole('link', { name: 'В список пользователей' }).click(); await expect(card(page)).toBeVisible(); expect(new URL(page.url()).searchParams.get('role')).toBe('admin')
})
for (const role of ['manager', 'storekeeper', 'order_operator']) {
  test(`F21 denies ${role} before users, roles or entry requests`, async ({ page }) => {
    await login(page, role, '/'); await expect(page.getByRole('heading', { name: 'Требуют внимания' })).toBeVisible()
    const reads: string[] = []; page.on('request', (request) => { if (/\/api\/v1\/(users|roles)/.test(request.url())) reads.push(request.url()) })
    for (const path of ['/admin/users', '/admin/users/new', `/admin/users/${id}/edit`, `/admin/users/${id}/password`]) { await page.goto(path); await expect(page.getByRole('heading', { name: 'Недостаточно прав' })).toBeVisible(); await expect(page.getByRole('article')).toHaveCount(0) }
    expect(reads).toEqual([])
  })
}
