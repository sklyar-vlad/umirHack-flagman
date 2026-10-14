import { useId, type ComponentPropsWithRef, type ReactNode } from 'react'
import { icons, type IconName } from '@/shared/assets/icons'
import { DateTimeInput } from './date-time-input'

export function Icon({ name }: { name: IconName }) {
  return <img className="ui-icon" src={icons[name]} alt="" aria-hidden="true" />
}

export function Button({ variant = 'primary', loading = false, loadingLabel = 'Подождите…', disabled, children, className = '', type = 'button', ...props }: ComponentPropsWithRef<'button'> & {
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost'
  loading?: boolean
  loadingLabel?: string
}) {
  return <button {...props} type={type} className={`ui-button ui-button--${variant} ${className}`} disabled={disabled || loading} aria-busy={loading || undefined}>
    {loading ? <><span className="ui-spinner" aria-hidden="true" />{loadingLabel}</> : children}
  </button>
}

export function Input({ className = '', ...props }: ComponentPropsWithRef<'input'>) {
  if (props.type === 'datetime-local') return <DateTimeInput {...props} className={className} />
  return <input {...props} className={`ui-input ${className}`} />
}

export function Select({ className = '', ...props }: ComponentPropsWithRef<'select'>) {
  return <select {...props} className={`ui-select ${className}`} />
}

export function Textarea({ className = '', ...props }: ComponentPropsWithRef<'textarea'>) {
  return <textarea {...props} className={`ui-input ui-textarea ${className}`} />
}

type FieldControlProps = {
  id: string
  'aria-describedby'?: string
  'aria-invalid'?: true
}

/** Render props bind the label, hint and validation error to any native control. */
export function FormField({ id, label, hint, error, required, children }: {
  id?: string
  label: ReactNode
  hint?: string
  error?: string
  required?: boolean
  children: (props: FieldControlProps) => ReactNode
}) {
  const generatedId = useId()
  const controlId = id ?? generatedId
  const describedBy = [hint && `${controlId}-hint`, error && `${controlId}-error`].filter(Boolean).join(' ') || undefined
  return <div className="ui-field">
    <label className="ui-field-label" htmlFor={controlId}>{label}{required && <span aria-hidden="true"> *</span>}</label>
    {children({ id: controlId, 'aria-describedby': describedBy, 'aria-invalid': error ? true : undefined })}
    {hint && <p id={`${controlId}-hint`} className="text-caption text-muted">{hint}</p>}
    {error && <p id={`${controlId}-error`} className="ui-field-error" role="alert">{error}</p>}
  </div>
}

export function Checkbox({ children, className = '', ...props }: Omit<ComponentPropsWithRef<'input'>, 'type'>) {
  return <label className={`ui-check ${className}`}><input {...props} type="checkbox" /> <span>{children}</span></label>
}

export function Switch({ children, className = '', ...props }: Omit<ComponentPropsWithRef<'input'>, 'type' | 'role'>) {
  return <label className={`ui-check ui-switch ${className}`}><input {...props} type="checkbox" role="switch" /><span className="ui-switch-track" aria-hidden="true" /><span>{children}</span></label>
}
