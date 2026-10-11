import { useSelector } from 'react-redux'
import { Navigate, useSearchParams } from 'react-router'
import { selectSession, visibleSections } from '@/entities/session'
import { procurementSearchParams, readProcurementSearch } from '@/entities/procurement'
export function RecommendationsLanding() {
  const [params] = useSearchParams(), role = useSelector(selectSession).user!.role
  return params.get('tab') === 'procurement' ? <Navigate replace to={`/recommendations/procurement?${procurementSearchParams(readProcurementSearch(params))}`} /> : <Navigate replace to={visibleSections(role, 'recommendations')[0]?.path ?? '/forbidden'} />
}
