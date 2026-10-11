import { z } from 'zod'
export const loginSchema = z.object({
  email: z.string().trim().pipe(z.email('Введите корректный email.')),
  password: z.string().min(1, 'Введите пароль.'),
})
export function retryAfterDeadline(value: string | undefined, now = Date.now()): number {
  if (!value) return now
  if (/^\d+$/.test(value.trim())) return now + Number(value) * 1000
  const date = Date.parse(value)
  return Number.isFinite(date) ? Math.max(now, date) : now
}
