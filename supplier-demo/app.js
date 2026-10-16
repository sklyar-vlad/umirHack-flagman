'use strict'
const element = (id) => document.getElementById(id)
let products = [], endpoint = '', connectedToken = '', busy = false
const uuid = () => crypto.randomUUID()
const unitNames = { g:'г',ml:'мл',pcs:'шт.' }
function status(id,message,error = false) { const node = element(id); node.textContent = message; node.dataset.error = String(error) }
function lock(value) { busy = value; document.querySelectorAll('button,input,select').forEach((button) => { button.disabled = value }) }
async function request(path,body) {
  const response = await fetch(`${endpoint}/api/v1/integration${path}`,{ method:body ? 'POST':'GET', credentials:'omit', cache:'no-store', headers:{ Authorization:`Bearer ${connectedToken}`,...(body ? {'Content-Type':'application/json'} : {}) }, ...(body ? {body:JSON.stringify(body)} : {}), signal:AbortSignal.timeout(15000) })
  const data = await response.json().catch(() => null)
  if (!response.ok) throw new Error(response.status === 401 ? 'Токен недействителен или отозван. Проверьте подключение.' : response.status === 409 ? 'Внешний ID уже использован с другим содержимым, либо продукт с таким названием существует. Для нового документа укажите новый ID.' : response.status === 429 ? 'Слишком много запросов. Подождите и повторите.' : data?.error?.message ?? `Запрос не выполнен (HTTP ${response.status}). Проверьте поля.`)
  if (!data) throw new Error('Сервер вернул некорректный ответ')
  return data
}
async function catalog(path) {
  const items = []
  for (let page=1;page<=1000;page++) {
    const data = await request(`${path}?page=${page}&page_size=100`)
    if (!Array.isArray(data.items) || !Number.isSafeInteger(data.total)) throw new Error('Некорректный справочник')
    items.push(...data.items)
    if (items.length >= data.total || data.items.length === 0) return items
  }
  throw new Error('Справочник слишком большой для демонстрационного клиента')
}
function option(value,label) { const node=document.createElement('option');node.value=value;node.textContent=label;return node }
function fillProducts(select) { const previous = select.value; select.replaceChildren(option('','Выберите продукт'),...products.filter((product) => product.is_active).map((product) => option(product.id,`${product.name} (${unitNames[product.unit]})`))); select.value = previous }
function localDate(instant) { const date = new Date(instant); return `${date.getFullYear()}-${String(date.getMonth()+1).padStart(2,'0')}-${String(date.getDate()).padStart(2,'0')}T${String(date.getHours()).padStart(2,'0')}:${String(date.getMinutes()).padStart(2,'0')}` }
function addLine() {
  if (element('lines').children.length >= 100) return
  const row=document.createElement('div');row.className='supply-line'
  for (const [name,label,type] of [['product','Продукт','select'],['quantity','Количество в г / мл / шт.','text'],['expiry','Годен до','datetime-local']]) {
    const wrapper=document.createElement('label');wrapper.textContent=label
    const control=document.createElement(type === 'select' ? 'select':'input');control.dataset.field=name;control.required=true
    if (type === 'select') fillProducts(control)
    else { control.type=type; if (name==='quantity') { control.inputMode='decimal';control.placeholder='Например, 1200' } else control.value=localDate(Date.now()+2*86400000) }
    wrapper.append(control);row.append(wrapper)
  }
  const remove=document.createElement('button');remove.type='button';remove.className='secondary';remove.textContent='Убрать';remove.addEventListener('click',() => { if(element('lines').children.length>1)row.remove() });row.append(remove);element('lines').append(row)
}
element('connection').addEventListener('submit',async (event) => {
  event.preventDefault();if(busy)return;lock(true);status('connection-status','Подключаем справочники…')
  try {
    const url=new URL(element('api').value.trim())
    if (!['http:','https:'].includes(url.protocol) || url.username || url.password || url.pathname !== '/' || url.search || url.hash) throw new Error('Укажите только адрес приложения: схема и домен, без пути и параметров.')
    endpoint=url.origin;connectedToken=element('token').value.trim()
    const categories=await catalog('/product-categories');products=await catalog('/products')
    element('category').replaceChildren(option('','Выберите категорию'),...categories.map((category) => option(category.id,category.name)))
    document.querySelectorAll('[data-field=product]').forEach(fillProducts)
    element('workspace').hidden=false;status('connection-status',`Подключено · продуктов: ${products.length}`)
  } catch(error) { element('workspace').hidden=true;connectedToken='';status('connection-status',error.message,true) }
  finally {lock(false)}
})
for (const id of ['api','token']) element(id).addEventListener('input',() => { connectedToken='';element('workspace').hidden=true;status('connection-status','Подключите справочники после изменения доступа') })
element('product-form').addEventListener('submit',async(event) => {
  event.preventDefault();if(busy)return;lock(true);status('product-status','Создаём продукт…')
  try {
    const unit=element('unit').value
    const product=await request('/products',{ external_id:element('product-external').value.trim(),product:{name:element('product-name').value.trim(),category_id:element('category').value,unit,display_unit:unit} })
    products=await catalog('/products');document.querySelectorAll('[data-field=product]').forEach(fillProducts)
    status('product-status',`Продукт «${product.name}» доступен в ресторане. Теперь его можно добавить в поставку.`)
  } catch(error) {status('product-status',`${error.message} При потере связи повторите с тем же внешним ID и теми же полями.`,true)}
  finally {lock(false)}
})
element('supply-form').addEventListener('submit',async(event) => {
  event.preventDefault();if(busy)return;lock(true);status('supply-status','Отправляем поставку…')
  try {
    const items=[...element('lines').children].map((row) => {
      const value=(field) => row.querySelector(`[data-field=${field}]`).value
      const quantity=value('quantity').trim().replace(',','.')
      if(!/^\d{1,11}(?:\.\d{1,3})?$/.test(quantity)||!/[1-9]/.test(quantity))throw new Error('Укажите положительное количество, не более трёх знаков после запятой.')
      const product=products.find((product) => product.id===value('product'))
      if(!product)throw new Error('Выберите продукт из справочника.')
      if(product.unit==='pcs'&&!/^\d+(?:\.0+)?$/.test(quantity))throw new Error('Штучные продукты принимаются только целыми количествами.')
      return {product_id:product.id,quantity,expires_at:new Date(value('expiry')).toISOString()}
    })
    const doc=await request('/supplies',{external_id:element('supply-external').value.trim(),number:element('supply-number').value.trim(),items})
    const labels={pending:'Ожидает приёмки',partial:'Принята частично',received:'Принята полностью'}
    status('supply-status',`Поставка №${doc.number} · ${labels[doc.status] ?? doc.status}. Документ ${doc.id}. Остатки меняются только после подтверждения сотрудником.`)
    element('incoming-link').href=`${endpoint}/supplies/incoming`;element('incoming-link').hidden=false
  } catch(error) {status('supply-status',`${error.message} При потере связи повторите с тем же внешним ID и теми же полями.`,true)}
  finally {lock(false)}
})
element('add-line').addEventListener('click',addLine)
element('new-product').addEventListener('click',() => {element('product-external').value=uuid();element('product-name').value='';status('product-status','')})
element('new-supply').addEventListener('click',() => {element('supply-external').value=uuid();element('supply-number').value='';element('lines').replaceChildren();addLine();status('supply-status','');element('incoming-link').hidden=true})
element('product-external').value=uuid();element('supply-external').value=uuid();element('zone').textContent=`Часовой пояс ввода сроков: ${Intl.DateTimeFormat().resolvedOptions().timeZone}. Количество указывается в базовых единицах продукта.`;addLine()
