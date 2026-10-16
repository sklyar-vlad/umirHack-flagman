import { useRef, useState } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { useProductCategoriesQuery } from '@/entities/inventory'
import { CategoryDialog } from '@/features/manage-category'
import { UnsavedChangesGuard } from '@/features/unsaved-changes'
import { ApiAccessBoundary } from '@/features/access'
import { Button, ErrorState, FormField, Input, Select, Skeleton } from '@/shared/ui'
import styles from './ProductCategoryDraft.module.css'

export function ProductCategoryDraft() {
  const { register, setValue, control, formState: { isDirty } } = useForm({ defaultValues: { name: '', category_id: '' } })
  const [open, setOpen] = useState(false), [categoryDirty, setCategoryDirty] = useState(false), [notice, setNotice] = useState('')
  const initiator = useRef<HTMLButtonElement | null>(null)
  const choices = useProductCategoriesQuery(undefined, { refetchOnMountOrArgChange: true }), categoryId = useWatch({ control, name: 'category_id' })
  return <section aria-label="Черновик продукта" className={styles.draft}>
    <UnsavedChangesGuard dirty={isDirty || categoryDirty} />
    <h2>Подготовка продукта</h2><p>Название и категория сохраняются в открытом черновике при создании категории. Сохранение продукта будет доступно в F17.</p>
    <FormField label="Название продукта">{(props) => <Input {...props} {...register('name')} />}</FormField>
    <ApiAccessBoundary error={choices.error}>
      {choices.isFetching && !choices.currentData && <Skeleton label="Загрузка вариантов категорий" />}
      {choices.error && <ErrorState title="Не удалось загрузить варианты категорий" onRetry={() => { void choices.refetch() }} />}
      <FormField label="Категория продукта">{(props) => <Select {...props} {...register('category_id')} value={categoryId} disabled={(choices.isFetching && !choices.currentData) || !!choices.error}><option value="">Выберите категорию</option>{categoryId && !choices.currentData?.some((category) => category.id === categoryId) && <option value={categoryId}>Выбранная категория · обновление справочника</option>}{choices.currentData?.map((category) => <option key={category.id} value={category.id}>{category.name}</option>)}</Select>}</FormField>
      <Button ref={initiator} variant="secondary" onClick={() => setOpen(true)}>Создать категорию для продукта</Button>
    </ApiAccessBoundary>
    {notice && <p role="status">{notice}</p>}
    {open && <CategoryDialog returnFocusRef={initiator} onDirty={setCategoryDirty} onClose={() => { setOpen(false); setCategoryDirty(false) }} onSaved={(category) => { setValue('category_id', category.id, { shouldDirty: true }); setOpen(false); setCategoryDirty(false); setNotice(`Выбрана категория: ${category.name}. Черновик продукта сохранён.`) }} />}
  </section>
}
