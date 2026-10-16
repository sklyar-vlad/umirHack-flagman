import { useRef } from 'react'
import { useDishQuery } from '@/entities/dish'
import { ApiAccessBoundary } from '@/features/access'
import { isApiError } from '@/shared/api'
import { formatQuantity } from '@/shared/lib/quantity'
import { Dialog, EmptyState, ErrorState, Skeleton } from '@/shared/ui'
export function RecipeDialog({ id, name, returnFocusTo, onClose }: { id: string; name: string; returnFocusTo: HTMLElement; onClose: () => void }) {
  const returnFocusRef = useRef(returnFocusTo)
  const query = useDishQuery(id, { refetchOnMountOrArgChange: true }), dish = query.currentData
  return <Dialog open title={`Состав: ${name}`} onClose={onClose} returnFocusRef={returnFocusRef} description="Актуальная рецептура на одну порцию. Просмотр не резервирует продукты.">
    {query.isFetching && !query.currentData && <Skeleton label="Загрузка состава" rows={3} />}
    <ApiAccessBoundary error={query.error}>
      {isApiError(query.error) && query.error.status === 404 ? <EmptyState title="Блюдо не найдено" description="Обновите каталог." /> : query.error && <ErrorState title="Не удалось загрузить состав" onRetry={() => { void query.refetch() }} retrying={query.isFetching} />}
      {!query.error && dish && <><p>Версия рецептуры: {dish.recipe_version ?? 'отсутствует'}. Ревизия блюда: {dish.revision}.</p>
        {!dish.is_active && <p>Блюдо отключено.</p>}
        {dish.recipe.length ? <ul>{dish.recipe.map((item) => <li key={item.product_id}>{item.product_name} — {formatQuantity(item.quantity_per_portion, item.unit)} на порцию</li>)}</ul> : <EmptyState title="Рецептура отсутствует" />}
        <p>Доступно отдельно: {dish.available_portions} порц.</p>
      </>}
    </ApiAccessBoundary>
  </Dialog>
}
