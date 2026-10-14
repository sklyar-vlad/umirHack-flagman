import { useRef, useState, type ChangeEvent, type ComponentPropsWithRef } from 'react'
import { Dialog } from './dialog'

const weekdays = ['Пн', 'Вт', 'Ср', 'Чт', 'Пт', 'Сб', 'Вс']
function calendarDate(value: string) { const [year, month, day] = value.split('-').map(Number); return new Date(Date.UTC(year, month - 1, day, 12)) }
const isoDay = (value: Date) => value.toISOString().slice(0, 10)

/** Native value/ref keeps RHF and the existing timezone conversion contract intact. */
export function DateTimeInput({ ref, className = '', ...props }: ComponentPropsWithRef<'input'>) {
  const input = useRef<HTMLInputElement>(null), trigger = useRef<HTMLButtonElement>(null), selected = useRef<HTMLButtonElement>(null)
  const [open, setOpen] = useState(false), [date, setDate] = useState(''), [time, setTime] = useState('00:00'), [month, setMonth] = useState('2000-01-01')
  const current = calendarDate(month), year = current.getUTCFullYear(), monthIndex = current.getUTCMonth()
  const firstWeekday = (current.getUTCDay() + 6) % 7, days = new Date(Date.UTC(year, monthIndex + 1, 0)).getUTCDate()
  function begin() {
    const value = input.current?.value ?? '', day = value.slice(0, 10) || isoDay(new Date())
    setDate(day); setTime(value.slice(11, 16) || '00:00'); setMonth(day.slice(0, 7) + '-01'); setOpen(true)
  }
  function apply() {
    const element = input.current
    if (!element || !date || !/^\d{2}:\d{2}$/.test(time)) return
    const next = `${date}T${time}`
    element.value = next
    props.onChange?.({ target: element, currentTarget: element, type: 'change' } as ChangeEvent<HTMLInputElement>)
    setOpen(false)
  }
  function shiftMonth(delta: number) { setMonth(isoDay(new Date(Date.UTC(year, monthIndex + delta, 1, 12)))) }
  return <div className="ui-datetime">
    <input {...props} lang="ru" type="datetime-local" step={props.step ?? 60} className={`ui-input ${className}`} ref={(node) => {
      input.current = node
      if (typeof ref === 'function') ref(node)
      else if (ref) ref.current = node
    }} />
    <button type="button" ref={trigger} className="ui-button ui-button--secondary" disabled={props.disabled || props.readOnly} aria-label="Выбрать дату и время" aria-haspopup="dialog" aria-expanded={open} onClick={begin}>Календарь</button>
    {open && <Dialog open title="Выберите дату и время" onClose={() => setOpen(false)} initialFocusRef={selected} returnFocusRef={trigger} actions={<>
      <button type="button" className="ui-button ui-button--secondary" onClick={() => setOpen(false)}>Отмена</button>
      <button type="button" className="ui-button ui-button--primary" disabled={!time} onClick={apply}>Применить</button>
    </>}>
      <div className="ui-calendar-heading">
        <button type="button" className="ui-button ui-button--ghost" aria-label="Предыдущий месяц" onClick={() => shiftMonth(-1)}>←</button>
        <p aria-live="polite">{new Intl.DateTimeFormat('ru-RU', { month: 'long', year: 'numeric', timeZone: 'UTC' }).format(current)}</p>
        <button type="button" className="ui-button ui-button--ghost" aria-label="Следующий месяц" onClick={() => shiftMonth(1)}>→</button>
      </div>
      <div className="ui-calendar-grid" role="group" aria-label="Дни месяца" onKeyDown={(event) => {
        const origin = (event.target as HTMLElement).dataset.date
        if (!origin || !['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End'].includes(event.key)) return
        event.preventDefault()
        const day = calendarDate(origin), weekday = (day.getUTCDay() + 6) % 7
        const delta = ({ ArrowLeft: -1, ArrowRight: 1, ArrowUp: -7, ArrowDown: 7, Home: -weekday, End: 6 - weekday })[event.key as 'ArrowLeft']
        day.setUTCDate(day.getUTCDate() + delta)
        const next = isoDay(day)
        if (next.slice(0, 7) !== month.slice(0, 7)) setMonth(next.slice(0, 7) + '-01')
        const grid = event.currentTarget
        requestAnimationFrame(() => grid.querySelector<HTMLButtonElement>(`[data-date="${next}"]`)?.focus())
      }}>
        {weekdays.map((day) => <span key={day} className="text-caption text-muted">{day}</span>)}
        {Array.from({ length: firstWeekday }, (_, index) => <span key={`blank-${index}`} aria-hidden="true" />)}
        {Array.from({ length: days }, (_, index) => {
          const day = isoDay(new Date(Date.UTC(year, monthIndex, index + 1, 12))), chosen = day === date
          return <button type="button" key={day} data-date={day} ref={chosen ? selected : undefined} aria-pressed={chosen} aria-label={new Intl.DateTimeFormat('ru-RU', { dateStyle: 'full', timeZone: 'UTC' }).format(calendarDate(day))} className={`ui-button ui-button--${chosen ? 'primary' : 'ghost'}`} onClick={() => setDate(day)}>{index + 1}</button>
        })}
      </div>
      <label className="ui-calendar-time">Время<input className="ui-input" type="time" step="60" value={time} onChange={(event) => setTime(event.target.value)} onKeyDown={(event) => {
        if (event.key === 'Enter') { event.preventDefault(); event.stopPropagation(); apply() }
      }} /></label>
      <p className="text-caption text-muted">{new Intl.DateTimeFormat('ru-RU', { dateStyle: 'long', timeZone: 'UTC' }).format(calendarDate(date || month))}, {time || 'укажите время'}</p>
    </Dialog>}
  </div>
}
