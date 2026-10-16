import { useState } from 'react'
import { useIntegrationTokensQuery,useCreateIntegrationTokenMutation,useRevokeIntegrationTokenMutation } from '@/entities/integration'
import { ApiAccessBoundary } from '@/features/access'
import { Badge, Button, Dialog, EmptyState, ErrorState, FormField, Input, Skeleton } from '@/shared/ui'
export function IntegrationsPage() {
  const query = useIntegrationTokensQuery(undefined,{ refetchOnMountOrArgChange:true }), [create,creating] = useCreateIntegrationTokenMutation(), [revoke,revoking] = useRevokeIntegrationTokenMutation()
  const [name,setName] = useState(''), [token,setToken] = useState(''), [error,setError] = useState(''), [confirm,setConfirm] = useState<{ id:string;name:string }>()
  return <section className="ui-form">
    <header><h1>Интеграции с поставщиками</h1><p className="text-muted">Отдельный API-токен для каждого внешнего сервиса. Он разрешает читать справочники, создавать продукты и отправлять заявки на поставку. Приёмку подтверждает сотрудник в разделе «Поставки».</p></header>
    <form className="ui-form" onSubmit={(event) => { event.preventDefault();setError('');void create({name:name.trim()}).unwrap().then((result) => {setToken(result.token);setName('');creating.reset()}).catch(() => setError('Не удалось создать токен. Проверьте название и повторите.')) }}>
      <FormField label="Название внешнего сервиса">{(props) => <Input {...props} value={name} required maxLength={100} placeholder="Например: поставщик овощей" onChange={(event) => setName(event.target.value)} />}</FormField>
      <div><Button type="submit" loading={creating.isLoading} disabled={!name.trim()}>Создать API-токен</Button></div>
    </form>
    {error && <p role="alert">{error}</p>}
    <ApiAccessBoundary error={query.error}>
      {query.isFetching && !query.currentData && <Skeleton rows={3} label="Загрузка интеграций" />}
      {query.error && <ErrorState title="Не удалось загрузить интеграции" onRetry={() => {void query.refetch()}} />}
      {query.currentData?.map((item) => <article key={item.id} className="ui-panel"><div className="ui-actions"><strong>{item.name}</strong><Badge tone={item.revoked_at ? 'neutral' : 'success'}>{item.revoked_at ? 'Токен отозван' : 'Активен'}</Badge>{!item.revoked_at && <Button variant="secondary" disabled={revoking.isLoading} onClick={() => setConfirm(item)}>Отозвать токен</Button>}</div></article>)}
      {query.currentData?.length === 0 && <EmptyState title="Интеграций пока нет" description="Создайте токен и передайте его внешнему сервису." />}
    </ApiAccessBoundary>
    {token && <Dialog open title="API-токен создан" onClose={() => setToken('')}><p>Скопируйте токен сейчас: после закрытия он больше не отображается.</p><FormField label="Новый API-токен">{(props) => <Input {...props} readOnly type="text" value={token} onFocus={(event) => event.target.select()} />}</FormField><Button onClick={() => setToken('')}>Токен сохранён</Button></Dialog>}
    {confirm && <Dialog open title="Отозвать API-токен?" onClose={() => {if(!revoking.isLoading)setConfirm(undefined)}}><p>Сервис «{confirm.name}» потеряет доступ. Уже отправленные поставки останутся в очереди приёмки.</p><div className="ui-actions"><Button disabled={revoking.isLoading} onClick={() => {setError('');void revoke(confirm.id).unwrap().then(() => setConfirm(undefined)).catch(() => setError('Не удалось отозвать токен. Повторите попытку.'))}}>Отозвать</Button><Button variant="ghost" disabled={revoking.isLoading} onClick={() => setConfirm(undefined)}>Отмена</Button></div></Dialog>}
  </section>
}
