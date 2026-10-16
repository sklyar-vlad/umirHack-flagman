import { useDishQuery, type HotOffer } from '@/entities/dish'
import { isApiError } from '@/shared/api'
import { ApiAccessBoundary } from '@/features/access'
import { formatQuantity } from '@/shared/lib/quantity'
import { Dialog, EmptyState, ErrorState, Skeleton } from '@/shared/ui'
export function DishCompositionDialog({ offer, present, onClose }: { offer: HotOffer; present: boolean; onClose: () => void }) {
  const query = useDishQuery(offer.dish.id, { refetchOnMountOrArgChange: true })
  const dish = query.currentData
  return <Dialog open title={`Состав: ${offer.dish.name}`} onClose={onClose} description="Просмотр состава не добавляет блюдо в заказ и не резервирует продукты.">
    {!present && <p role="status">Предложение больше не актуально. Состав показан отдельно от рекомендаций.</p>}
    {query.isFetching && !query.currentData && <Skeleton label="Загрузка состава" rows={3} />}
    <ApiAccessBoundary error={query.error}>
      {isApiError(query.error) && query.error.status === 404 ? <EmptyState title="Блюдо не найдено" description="Возможно, блюдо удалено. Обновите предложения." /> : query.error && <ErrorState title="Не удалось загрузить состав" onRetry={() => { void query.refetch() }} retrying={query.isFetching} />}
      {!query.error && dish && <><p>Текущая рецептура: {dish.recipe_version ?? 'отсутствует'}.</p>
        {dish.recipe_version !== offer.dish.recipe_version && <p role="status">Рецептура изменилась после снимка рекомендаций. Перед добавлением обновите предложения.</p>}
        {!dish.is_active && <p role="status">Блюдо отключено.</p>}
        {dish.recipe.length ? <ul>{dish.recipe.map((item) => <li key={item.product_id}>{item.product_name} — {formatQuantity(item.quantity_per_portion, item.unit)} на порцию</li>)}</ul> : <EmptyState title="Рецептура отсутствует" />}
        <p>Отдельно доступно: {dish.available_portions} порц. Общую корзину проверяет сервер.</p>
      </>}
    </ApiAccessBoundary>
  </Dialog>
}
