import type { OrderCheck, OrderRefusal } from '@/entities/order'
import { formatQuantity } from '@/shared/lib/quantity'
import { DataTable } from '@/shared/ui'

export function OrderRequirements({ result }: { result: Pick<OrderCheck | OrderRefusal, 'requirements' | 'shortages' | 'issues'> }) {
  return <section aria-label="Потребность всей корзины"><h3>Потребность всей корзины</h3>
    <DataTable caption="Продукты для всей корзины" rows={result.requirements} rowKey={(row) => row.product_id} emptyState={<p>Сервер не рассчитал потребность. Проверьте причины недоступности.</p>} columns={[
      { key: 'name', header: 'Продукт', render: (row) => row.product_name },
      { key: 'required', header: 'Требуется', render: (row) => formatQuantity(row.required_quantity, row.unit) },
      { key: 'available', header: 'Доступно', render: (row) => formatQuantity(row.available_quantity, row.unit) },
      { key: 'missing', header: 'Недостаёт', render: (row) => formatQuantity(row.missing_quantity, row.unit) },
    ]} />
    {result.shortages.map((row) => <p key={row.product_id}>{row.product_name}: недостаёт {formatQuantity(row.missing_quantity, row.unit)}. Просрочено: {formatQuantity(row.expired_quantity, row.unit)}.{row.reason_code === 'only_expired_stock' && ' Весь физический остаток просрочен.'}</p>)}
    {result.issues.map((issue) => <p key={`${issue.dish_id}-${issue.code}`} role="alert">{issue.message}{issue.current_recipe_version !== null && ` Актуальная версия: ${issue.current_recipe_version}.`}</p>)}
  </section>
}
