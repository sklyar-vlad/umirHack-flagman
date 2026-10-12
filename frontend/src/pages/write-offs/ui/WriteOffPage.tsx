import { Link } from 'react-router'
import { EmptyState } from '@/shared/ui'
export function WriteOffPage() {
  return <><h1>Ручное списание</h1><EmptyState title="Выберите партию на складе" description="Раскройте партии нужного продукта и нажмите «Списать партию». Можно списать годный или просроченный физический остаток." action={<Link to="/inventory">Открыть склад</Link>} /></>
}
