// @vitest-environment jsdom
import { createRef } from 'react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Button, Checkbox, DataTable, Dialog, Drawer, EmptyState, ErrorState, FormField, Input, NoResults, Notification, Pagination, Select, Skeleton, Switch, Textarea } from './index'
import { paginationItems } from './pagination'

afterEach(cleanup)
beforeAll(() => {
  HTMLDialogElement.prototype.showModal = function () { this.open = true }
  HTMLDialogElement.prototype.close = function () { this.open = false }
})

describe('shared form controls', () => {
  it('does not submit by default and prevents callbacks while loading or disabled', async () => {
    const user = userEvent.setup()
    const action = vi.fn()
    const submit = vi.fn((event) => event.preventDefault())
    const view = render(<form onSubmit={submit}><Button onClick={action}>Действие</Button></form>)
    await user.click(screen.getByRole('button', { name: 'Действие' }))
    expect(action).toHaveBeenCalledTimes(1)
    expect(submit).not.toHaveBeenCalled()
    view.rerender(<Button loading onClick={action}>Действие</Button>)
    const loading = screen.getByRole('button', { name: 'Подождите…' }) as HTMLButtonElement
    expect(loading.disabled).toBe(true)
    expect(loading.getAttribute('aria-busy')).toBe('true')
    await user.click(loading)
    view.rerender(<Button disabled onClick={action}>Действие</Button>)
    await user.click(screen.getByRole('button'))
    expect(action).toHaveBeenCalledTimes(1)
  })

  it('connects unique labels, hints and dynamic errors to controls and preserves refs', async () => {
    const ref = createRef<HTMLInputElement>()
    const field = (error?: string) => <FormField label="Название" hint="Два символа" error={error}>{(props) => <Input {...props} ref={ref} />}</FormField>
    const view = render(<>{field()}<FormField label="Комментарий">{(props) => <Textarea {...props} />}</FormField></>)
    const input = screen.getByLabelText('Название') as HTMLInputElement
    expect(ref.current).toBe(input)
    expect(input.id).not.toBe(screen.getByLabelText('Комментарий').id)
    expect(document.getElementById(input.getAttribute('aria-describedby')!)?.textContent).toBe('Два символа')
    await userEvent.setup().type(input, 'Сыр')
    expect(input.value).toBe('Сыр')
    view.rerender(field('Слишком коротко'))
    expect(screen.getByLabelText('Название').getAttribute('aria-invalid')).toBe('true')
    const ids = screen.getByLabelText('Название').getAttribute('aria-describedby')!.split(' ')
    expect(ids.map((id) => document.getElementById(id)?.textContent)).toEqual(['Два символа', 'Слишком коротко'])
    expect(screen.getByRole('alert').textContent).toBe('Слишком коротко')
  })

  it('uses native select and keyboard-operable checkboxes and switches', async () => {
    const user = userEvent.setup()
    render(<><FormField label="Категория">{(props) => <Select {...props}><option value="a">А</option><option value="b">Б</option></Select>}</FormField><Checkbox>Проверено</Checkbox><Switch>Уведомления</Switch><Checkbox disabled>Недоступный флажок</Checkbox></>)
    await user.selectOptions(screen.getByLabelText('Категория'), 'b')
    expect((screen.getByLabelText('Категория') as HTMLSelectElement).value).toBe('b')
    screen.getByRole('checkbox', { name: 'Проверено' }).focus()
    await user.keyboard(' ')
    expect((screen.getByRole('checkbox', { name: 'Проверено' }) as HTMLInputElement).checked).toBe(true)
    await user.tab()
    expect(document.activeElement).toBe(screen.getByRole('switch'))
    await user.keyboard(' ')
    expect((screen.getByRole('switch') as HTMLInputElement).checked).toBe(true)
    await user.click(screen.getByRole('checkbox', { name: 'Недоступный флажок' }))
    expect((screen.getByRole('checkbox', { name: 'Недоступный флажок' }) as HTMLInputElement).checked).toBe(false)
  })
})

describe('modal lifecycle', () => {
  it('labels modal, handles Escape/close, locks scroll and restores focus on unmount', async () => {
    const close = vi.fn()
    const previous = document.createElement('button')
    document.body.append(previous)
    previous.focus()
    document.body.style.overflow = 'auto'
    const view = render(<Dialog open title="Подтверждение" description="Проверьте данные" onClose={close}><Input aria-label="Данные" /></Dialog>)
    const modal = screen.getByRole('dialog', { name: 'Подтверждение' }) as HTMLDialogElement
    expect(modal.open).toBe(true)
    expect(document.getElementById(modal.getAttribute('aria-describedby')!)?.textContent).toBe('Проверьте данные')
    expect(document.body.style.overflow).toBe('hidden')
    fireEvent(modal, new Event('cancel', { cancelable: true }))
    expect(close).toHaveBeenCalledTimes(1)
    await userEvent.setup().click(screen.getByRole('button', { name: 'Закрыть' }))
    expect(close).toHaveBeenCalledTimes(2)
    view.unmount()
    expect(document.body.style.overflow).toBe('auto')
    expect(document.activeElement).toBe(previous)
    previous.remove()
    document.body.style.overflow = ''
  })

  it('opens drawer only when requested and focuses an explicit initial target', () => {
    const target = createRef<HTMLButtonElement>()
    const view = render(<Drawer open={false} title="Параметры" onClose={() => {}} initialFocusRef={target}><Button ref={target}>Применить</Button></Drawer>)
    expect(screen.queryByRole('dialog')).toBeNull()
    view.rerender(<Drawer open title="Параметры" onClose={() => {}} initialFocusRef={target}><Button ref={target}>Применить</Button></Drawer>)
    expect(document.activeElement).toBe(target.current)
  })
})

describe('records, pagination and states', () => {
  it('bounds pagination for large lists and rejects invalid ranges', () => {
    expect(paginationItems(50, 100000)).toEqual([1, 'start-gap', 49, 50, 51, 'end-gap', 100000])
    expect(paginationItems(1, 1)).toEqual([1])
    expect(paginationItems(3, 3)).toEqual([1, 2, 3])
    for (const args of [[0, 2], [3, 2], [1, 0], [1.1, 4]]) expect(() => paginationItems(...args as [number, number])).toThrow(RangeError)
  })

  it('enforces page boundaries, announces current page and blocks loading navigation', async () => {
    const user = userEvent.setup()
    const change = vi.fn()
    const view = render(<Pagination page={1} pageCount={3} onPageChange={change} />)
    expect((screen.getByRole('button', { name: 'Предыдущая страница' }) as HTMLButtonElement).disabled).toBe(true)
    expect(screen.getByRole('button', { name: 'Страница 1' }).getAttribute('aria-current')).toBe('page')
    await user.click(screen.getByRole('button', { name: 'Следующая страница' }))
    expect(change).toHaveBeenCalledWith(2)
    view.rerender(<Pagination page={3} pageCount={3} onPageChange={change} loading />)
    for (const button of screen.getAllByRole('button')) expect((button as HTMLButtonElement).disabled).toBe(true)
    await user.click(screen.getByRole('button', { name: 'Страница 2' }))
    expect(change).toHaveBeenCalledTimes(1)
  })

  it('provides table caption/headers, equivalent mobile labels and empty replacement', () => {
    const columns = [{ key: 'name', header: 'Продукт', render: (row: { name: string }) => row.name }]
    const view = render(<DataTable rows={[{ name: 'Сыр' }]} columns={columns} rowKey={(row) => row.name} caption="Остатки" emptyState={<EmptyState />} />)
    expect(screen.getByRole('table', { name: 'Остатки' })).toBeDefined()
    expect(screen.getByRole('columnheader').getAttribute('scope')).toBe('col')
    expect(screen.getByRole('region', { name: 'Остатки' }).textContent).toBe('ПродуктСыр')
    view.rerender(<DataTable rows={[]} columns={columns} rowKey={(row) => row.name} caption="Остатки" emptyState={<EmptyState />} />)
    expect(screen.queryByRole('table')).toBeNull()
    expect(screen.getByRole('heading', { name: 'Пока нет данных' })).toBeDefined()
  })

  it('announces notifications, dismisses explicitly and prevents repeated retries', async () => {
    const user = userEvent.setup()
    const dismiss = vi.fn()
    const retry = vi.fn()
    const reset = vi.fn()
    const view = render(<><Notification title="Сохранено" onDismiss={dismiss} /><NoResults query="Сыр" onReset={reset} /><ErrorState onRetry={retry} /><Skeleton label="Загрузка остатков" /></>)
    expect(screen.getByRole('status', { name: 'Загрузка остатков' }).getAttribute('aria-busy')).toBe('true')
    await user.click(screen.getByRole('button', { name: 'Скрыть уведомление: Сохранено' }))
    expect(dismiss).toHaveBeenCalledTimes(1)
    await user.click(screen.getByRole('button', { name: 'Сбросить фильтры' }))
    expect(reset).toHaveBeenCalledTimes(1)
    await user.click(screen.getByRole('button', { name: 'Попробовать снова' }))
    expect(retry).toHaveBeenCalledTimes(1)
    view.rerender(<ErrorState onRetry={retry} retrying />)
    await user.click(screen.getByRole('button', { name: 'Повторяем…' }))
    expect(retry).toHaveBeenCalledTimes(1)
    view.rerender(<Notification tone="danger" title="Ошибка сохранения" />)
    expect(screen.getByRole('alert').textContent).toBe('Ошибка сохранения')
  })
})
