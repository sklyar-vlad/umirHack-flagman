import { randomUUID } from 'node:crypto'
import { createServer, type Server } from 'node:http'
import { readFile } from 'node:fs/promises'
import { expect, test, type APIRequestContext, type Page } from '@playwright/test'

const origin = process.env.FRONTEND_TEST_ORIGIN!, password = process.env.FRONTEND_TEST_PASSWORD!
const pause = (ms: number) => new Promise((resolve) => setTimeout(resolve,ms))
class Client {
  csrf = ''
  readonly request: APIRequestContext
  constructor(request: APIRequestContext) { this.request = request }
  async call(path: string, method = 'GET', data?: unknown, bearer?: string, status = method === 'GET' ? 200 : 201) {
    for (let attempt=0;attempt<4;attempt++) {
      const result = await this.request.fetch(`/api/v1${path}`,{ method,headers: bearer ? { Authorization:`Bearer ${bearer}` } : { Origin:origin,'X-CSRF-Token':this.csrf,'Idempotency-Key':randomUUID() },...(data === undefined ? {} : {data}) })
      if (result.status() === 429 && attempt < 3) { await pause(1100);continue }
      expect(result.status(),`HTTP ${method} ${path}`).toBe(status)
      await pause(150)
      return status === 204 ? undefined : result.json()
    }
    throw new Error('Request retry budget exhausted')
  }
  async login(role = 'admin') {
    const response = await this.call('/auth/login','POST',{email:`${role === 'order_operator' ? 'operator' : role}@flagman.invalid`,password},undefined,200)
    this.csrf = response.csrf_token
  }
}
async function login(page: Page,role: string,target: string,email?: string) {
  await pause(1200)
  await page.goto(`/login?returnTo=${encodeURIComponent(target)}`)
  await page.getByLabel(/^Email/).fill(email ?? `${role === 'order_operator' ? 'operator' : role}@flagman.invalid`)
  await page.getByLabel(/^Пароль/).fill(password)
  await page.getByRole('button',{name:'Войти',exact:true}).click()
  await expect(page).toHaveURL(new URL(target,origin).pathname)
  await pause(1200)
  if (new URL(target,origin).search) await page.goto(target)
}
test.use({ actionTimeout:15_000 })
let supplier: Server, supplierOrigin: string
// This origin is separate from the application, so native browser CORS and bearer
// authentication are exercised by the actual standalone supplier page.
test.beforeAll(async () => {
  supplier = createServer(async (request,response) => {
    const file = ({ '/':'index.html','/index.html':'index.html','/app.js':'app.js','/styles.css':'styles.css' } as Record<string,string>)[request.url ?? '']
    if (!file) {response.writeHead(404);response.end();return}
    const content = await readFile(new URL(`../../../supplier-demo/${file}`,import.meta.url))
    response.writeHead(200,{'Content-Type':file.endsWith('.js')?'application/javascript':file.endsWith('.css')?'text/css':'text/html; charset=utf-8'});response.end(content)
  })
  await new Promise<void>((resolve) => supplier.listen(0,'127.0.0.1',resolve))
  const address = supplier.address()
  if (!address || typeof address === 'string') throw new Error('Supplier fixture failed')
  supplierOrigin = `http://127.0.0.1:${address.port}`
})
test.afterAll(async () => {if(supplier)await new Promise<void>((resolve,reject) => supplier.close((error) => error?reject(error):resolve()))})

test('external supplier → partial reception → order/write-off → product analytics, desktop and mobile',async({ page,request,browser }) => {
  test.setTimeout(180_000)
  const client = new Client(request);await client.login()
  const token = await client.call('/integration-tokens','POST',{name:`Поставщик ${randomUUID()}`})
  const sourceContext = await browser.newContext(), source = await sourceContext.newPage()
  try {
    await source.goto(supplierOrigin)
    await source.getByLabel('Адрес приложения').fill(origin)
    await source.getByLabel('API-токен',{exact:true}).fill(token.token)
    await source.getByRole('button',{name:'Подключить справочники'}).click()
    await expect(source.locator('#connection-status')).toContainText('Подключено')
    const productName = `Внешнее молоко ${randomUUID().slice(0,8)}`, number = `ВХ-${randomUUID().slice(0,8)}`
    await source.getByLabel('Название продукта').fill(productName)
    await source.getByLabel('Категория',{exact:true}).selectOption({index:1})
    await source.getByLabel('Единица учёта').selectOption('ml')
    await source.getByRole('button',{name:'Создать продукт в ресторане'}).click()
    await expect(source.locator('#product-status')).toContainText('доступен в ресторане')
    const catalog = await client.call(`/integration/products?search=${encodeURIComponent(productName)}`,'GET',undefined,token.token)
    const product = catalog.items.find((item: {name:string}) => item.name === productName)
    expect(!!product).toBe(true)
    await source.getByLabel('Номер накладной').fill(number)
    await source.locator('[data-field=product]').selectOption(product.id)
    await source.getByLabel('Количество в г / мл / шт.').fill('100')
    const managerEmail = `manager-${randomUUID()}@example.test`
    await client.call('/users','POST',{name:'Менеджер приёмки',email:managerEmail,role:'manager',password})
    await login(page,'manager','/supplies/incoming',managerEmail)
    await pause(1200)
    await source.getByRole('button',{name:'Отправить на приёмку'}).click()
    await expect(source.locator('#supply-status')).toContainText('Ожидает приёмки')
    const pending = await client.call(`/analytics/product-flow?product_id=${product.id}`)
    expect(pending.total).toBe(0)
    const card = page.getByRole('article').filter({has:page.getByRole('heading',{name:`Поставка №${number}`,exact:true})})
    await expect(card.getByText('Ожидает приёмки',{exact:true})).toBeVisible()
    await card.getByRole('button',{name:'Принять полностью или частично'}).click()
    await page.getByRole('dialog').getByRole('textbox').fill('40')
    await page.getByRole('button',{name:'Подтвердить приёмку'}).click()
    await expect(page.getByRole('dialog')).toHaveCount(0)
    await expect(card.getByText('Принята частично',{exact:true})).toBeVisible()
    await page.screenshot({path:'test-results/production/incoming-partial-desktop.png',fullPage:true})
    await card.getByRole('button',{name:'Принять полностью или частично'}).click()
    await expect(page.getByRole('dialog').getByRole('textbox')).toHaveValue('60')
    await page.getByRole('button',{name:'Подтвердить приёмку'}).click()
    await expect(card.getByText('Принята полностью',{exact:true})).toBeVisible()
    const dish = await client.call('/dishes','POST',{name:`Блюдо ${randomUUID()}`,recipe:[{product_id:product.id,quantity_per_portion:'10'}]})
    await client.call('/orders','POST',{items:[{dish_id:dish.id,portions:2,recipe_version:dish.recipe_version}]})
    const queue = await client.call('/incoming-supplies?page_size=100')
    const doc = queue.items.find((item: {number:string}) => item.number === number)
    const receipt = await client.call(`/supplies/${doc.receipt_ids[0]}`)
    await client.call('/write-offs','POST',{batch_id:receipt.items[0].batch.id,quantity:'5',reason:'Порча при приёмке'})
    await pause(1200)
    await page.goto(`/analytics/product-flow?product_id=${product.id}`)
    const row = page.locator('tbody tr').filter({hasText:productName})
    await expect(row).toContainText('100 мл');await expect(row).toContainText('20 мл');await expect(row).toContainText('5 мл')
    await page.screenshot({path:'test-results/production/product-flow-desktop.png',fullPage:true})
    await page.setViewportSize({width:390,height:1000})
    await expect(page.getByRole('heading',{name:'Движение продуктов'})).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({path:'test-results/production/product-flow-mobile.png',fullPage:true})
    await pause(1200)
    await page.goto('/supplies/incoming')
    await expect(page.getByRole('heading',{name:`Поставка №${number}`,exact:true})).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({path:'test-results/production/incoming-mobile.png',fullPage:true})
    await client.call(`/integration-tokens/${token.id}`,'DELETE',undefined,undefined,204)
  } finally {await sourceContext.close()}
})

test('manager edits manual offer and waiter sees only published offers and menu',async({ page,request,browser }) => {
  test.setTimeout(180_000)
  const client = new Client(request);await client.login()
  const categories = await client.call('/product-categories')
  const product = await client.call('/products','POST',{name:`Продукт предложения ${randomUUID()}`,category_id:categories.items[0].id,unit:'g',display_unit:'g'})
  await client.call('/supplies','POST',{number:`П-${randomUUID()}`,received_at:new Date(Date.now()-1000).toISOString(),items:[{product_id:product.id,quantity:'1000',expires_at:new Date(Date.now()+7*86400000).toISOString()}]})
  const dishName=`Предложение ${randomUUID().slice(0,8)}`
  await client.call('/dishes','POST',{name:dishName,recipe:[{product_id:product.id,quantity_per_portion:'10'}]})
  const managerEmail = `manager-${randomUUID()}@example.test`, waiterEmail = `waiter-${randomUUID()}@example.test`
  await client.call('/users','POST',{name:'Менеджер предложений',email:managerEmail,role:'manager',password})
  await client.call('/users','POST',{name:'Официант',email:waiterEmail,role:'order_operator',password})
  await login(page,'manager',`/dishes?search=${encodeURIComponent(dishName)}`,managerEmail)
  await page.getByRole('button',{name:'В горячие предложения',exact:true}).click()
  await expect(page.getByText(`${dishName}: добавлено в горячие предложения`,{exact:true})).toBeVisible()
  await pause(1200)
  await page.goto(`/recommendations/offers?search=${encodeURIComponent(dishName)}`)
  await page.getByRole('button',{name:'Редактировать предложение'}).click()
  await page.getByLabel('Рекомендация официанту').fill('Предлагать гостям к обеду')
  await page.getByRole('button',{name:'Сохранить',exact:true}).click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page.getByText('Предлагать гостям к обеду',{exact:true})).toBeVisible()
  await page.screenshot({path:'test-results/production/manager-offers-desktop.png',fullPage:true})
  const waiterContext=await browser.newContext({baseURL:origin}), waiter=await waiterContext.newPage()
  try {
    await login(waiter,'order_operator','/dishes',waiterEmail)
    await expect(waiter.getByRole('heading',{name:'Меню',exact:true})).toBeVisible()
    await expect(waiter.getByRole('button',{name:'В горячие предложения',exact:true})).toHaveCount(0)
    await pause(1200)
    await waiter.goto(`/recommendations/offers?search=${encodeURIComponent(dishName)}`)
    await expect(waiter.getByText('Предлагать гостям к обеду',{exact:true})).toBeVisible()
    await expect(waiter.getByRole('button',{name:'Редактировать предложение'})).toHaveCount(0)
    await waiter.setViewportSize({width:390,height:1000})
    expect(await waiter.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await waiter.screenshot({path:'test-results/production/waiter-offers-mobile.png',fullPage:true})
    await page.getByRole('button',{name:'Редактировать предложение'}).click()
    await page.getByLabel('Показывать официанту',{exact:true}).uncheck()
    await page.getByRole('button',{name:'Сохранить',exact:true}).click()
    await expect(page.getByRole('dialog')).toHaveCount(0)
    await expect(waiter.getByText('Предлагать гостям к обеду',{exact:true})).toHaveCount(0)
    await expect(waiter.getByRole('heading',{name:dishName,exact:true})).toHaveCount(0)
  } finally {await waiterContext.close()}
})
