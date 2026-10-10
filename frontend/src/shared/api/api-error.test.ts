import { describe, expect, it } from 'vitest'
import { normalizeApiError } from './api-error'

describe('API error contract', () => {
  it('preserves validation fields and request ID for a rejected form', () => {
    const fields = [{ path: 'quantity', code: 'positive', message: 'Количество должно быть положительным' }]
    const error = normalizeApiError({ status: 422, data: {
      error: { code: 'VALIDATION_ERROR', message: 'Проверьте форму', details: { fields } },
      request_id: 'form-request',
    } })
    expect(error).toMatchObject({ status: 422, code: 'VALIDATION_ERROR', request_id: 'form-request', details: { fields } })
  })

  it('keeps an HTTP status when a proxy returns HTML instead of JSON', () => {
    const error = normalizeApiError({ status: 'PARSING_ERROR', originalStatus: 502, data: '<html>upstream failure</html>', error: 'SyntaxError' })
    expect(error.status).toBe(502)
    expect(JSON.stringify(error)).not.toContain('<html>')
    expect(JSON.stringify(error)).not.toContain('SyntaxError')
  })

  it('hides internal server messages from the UI', () => {
    const error = normalizeApiError({ status: 500, data: {
      error: { code: 'INTERNAL_ERROR', message: 'database password=secret', details: {} },
      request_id: 'server-request',
    } })
    expect(error.message).not.toContain('secret')
    expect(error.request_id).toBe('server-request')
  })

  it.each(['FETCH_ERROR', 'TIMEOUT_ERROR'] as const)('normalizes %s without exposing the transport exception', (status) => {
    const error = normalizeApiError({ status, error: 'private transport exception' })
    expect(error.status).toBe(status === 'FETCH_ERROR' ? 'NETWORK_ERROR' : 'TIMEOUT_ERROR')
    expect(JSON.stringify(error)).not.toContain('private transport exception')
  })
})
