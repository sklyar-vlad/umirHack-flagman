// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { OrderFilters } from './OrderFilters'
import { SupplyFilters } from '@/pages/supplies/ui/SupplyFilters'
import { defaultHistorySearch } from '@/shared/lib/history'

afterEach(cleanup)
describe('date filter drafts after restaurant timezone change', () => {
  it.each(['orders', 'supplies'])('preserves draft instant until explicit review in %s', async (kind) => {
    const user = userEvent.setup(), onChange = vi.fn(), onReset = vi.fn()
    const view = (zone: string) => kind === 'orders' ? <OrderFilters values={{ ...defaultHistorySearch, reason_code: '' }} tab="orders" timezone={zone} onChange={onChange} onReset={onReset} /> : <SupplyFilters values={defaultHistorySearch} timezone={zone} onChange={onChange} onReset={onReset} />
    const rendered = render(view('Europe/Moscow'))
    await user.type(screen.getByLabelText('Создана с (включительно)'), '2026-10-12T12:00')
    rendered.rerender(view('Asia/Yekaterinburg'))
    expect(screen.getByLabelText('Создана с (включительно)')).toHaveProperty('value', '2026-10-12T12:00')
    expect(screen.getByRole('button', { name: 'Найти' })).toHaveProperty('disabled', true)
    await user.click(screen.getByRole('button', { name: 'Пересмотреть период в новой зоне' }))
    expect(screen.getByLabelText('Создана с (включительно)')).toHaveProperty('value', '2026-10-12T14:00')
    await user.click(screen.getByRole('button', { name: 'Найти' })); expect(onChange.mock.calls[0][0]).toMatchObject({ from: '2026-10-12T09:00:00Z' })
  })
  it('refuses a destination DST overlap without replacing the original filter draft', async () => {
    const user = userEvent.setup(), onChange = vi.fn(), onReset = vi.fn()
    const view = (timezone: string) => <OrderFilters values={{ ...defaultHistorySearch, reason_code: '' }} tab="orders" timezone={timezone} onChange={onChange} onReset={onReset} />
    const rendered = render(view('UTC')); await user.type(screen.getByLabelText('Создана с (включительно)'), '2026-11-01T05:30')
    rendered.rerender(view('America/New_York')); await user.click(screen.getByRole('button', { name: 'Пересмотреть период в новой зоне' }))
    expect(screen.getByLabelText('Создана с (включительно)')).toHaveProperty('value', '2026-11-01T05:30'); expect(screen.getByRole('button', { name: 'Найти' })).toHaveProperty('disabled', true); expect(onChange).not.toHaveBeenCalled()
    expect(screen.getByText(/Исходные даты сохранены/)).toBeTruthy()
  })
})
