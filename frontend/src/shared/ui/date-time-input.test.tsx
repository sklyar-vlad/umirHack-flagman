// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { useForm } from 'react-hook-form'
import { FormField, Input } from './controls'

beforeEach(() => {
  Object.defineProperty(HTMLDialogElement.prototype, 'showModal', { configurable: true, value: vi.fn(function (this: HTMLDialogElement) { this.open = true }) })
  Object.defineProperty(HTMLDialogElement.prototype, 'close', { configurable: true, value: vi.fn(function (this: HTMLDialogElement) { this.open = false }) })
})
afterEach(() => { cleanup(); vi.restoreAllMocks() })
it('applies a Russian calendar selection through RHF and restores trigger focus', async () => {
  const saved = vi.fn()
  function Form() {
    const { register, handleSubmit } = useForm({ defaultValues: { date: '2026-10-14T12:30:45' } })
    return <form onSubmit={handleSubmit(saved)}><FormField label="Дата приёмки">{(props) => <Input {...props} {...register('date')} type="datetime-local" step="any" />}</FormField><button type="submit">Сохранить</button></form>
  }
  render(<Form />)
  const trigger = screen.getByRole('button', { name: 'Выбрать дату и время' })
  trigger.focus(); fireEvent.click(trigger)
  expect((screen.getByRole('dialog') as HTMLDialogElement).open).toBe(true)
  fireEvent.click(screen.getByRole('button', { name: /15 октября 2026/ }))
  fireEvent.change(screen.getByLabelText('Время'), { target: { value: '16:45' } })
  fireEvent.click(screen.getByRole('button', { name: 'Применить' }))
  expect((screen.getByLabelText('Дата приёмки') as HTMLInputElement).value).toBe('2026-10-15T16:45')
  expect(document.activeElement).toBe(trigger)
  fireEvent.click(screen.getByRole('button', { name: 'Сохранить' }))
  await vi.waitFor(() => expect(saved.mock.calls[0]?.[0]).toEqual({ date: '2026-10-15T16:45' }))
})
it('cancels without losing the original fractional timestamp', () => {
  render(<Input aria-label="Период" type="datetime-local" defaultValue="2026-10-14T12:30:45.123" step="any" />)
  fireEvent.click(screen.getByRole('button', { name: 'Выбрать дату и время' }))
  fireEvent.click(screen.getByRole('button', { name: 'Отмена' }))
  expect((screen.getByLabelText('Период') as HTMLInputElement).value).toBe('2026-10-14T12:30:45.123')
})
