import { useState } from 'react'
import { useUpdateHotOfferMutation, type HotOffer } from '@/entities/dish'
import { Button, Checkbox, Dialog, FormField, Textarea } from '@/shared/ui'
export function OfferEditor({ item, onClose }: { item: HotOffer; onClose: () => void }) {
  const [published, setPublished] = useState(item.is_published ?? true), [manual, setManual] = useState(item.is_manual ?? false), [message, setMessage] = useState(item.message ?? ''), [error, setError] = useState('')
  const [save, mutation] = useUpdateHotOfferMutation()
  return <Dialog open title={`Предложение: ${item.dish.name}`} onClose={() => { if (!mutation.isLoading) onClose() }}>
    <form className="ui-form" onSubmit={(event) => {
      event.preventDefault(); setError('')
      void save({ id: item.dish.id, is_published: published, is_manual: manual, message, expected_revision: item.preference_revision ?? 0 }).unwrap().then(onClose).catch((error: { status?: number }) => setError(error.status === 409 ? 'Предложение уже изменено. Закройте окно, обновите список и повторите.' : 'Не удалось сохранить предложение. Повторите попытку.'))
    }}>
      <Checkbox checked={published} onChange={(event) => setPublished(event.target.checked)}>Показывать официанту</Checkbox>
      <Checkbox checked={manual} onChange={(event) => setManual(event.target.checked)}>Оставлять в предложениях по выбору менеджера</Checkbox>
      <FormField label="Рекомендация официанту" hint="Например: предлагать гостям в первую очередь">{(props) => <Textarea {...props} maxLength={500} value={message} onChange={(event) => setMessage(event.target.value)} />}</FormField>
      <p className="text-muted">Блюдо показывается только при наличии ингредиентов. Ручной выбор сохраняет предложение, пока блюдо доступно.</p>
      {error && <p role="alert">{error}</p>}
      <div className="ui-actions"><Button type="submit" loading={mutation.isLoading}>Сохранить</Button><Button variant="ghost" disabled={mutation.isLoading} onClick={onClose}>Отмена</Button></div>
    </form>
  </Dialog>
}
