// @vitest-environment jsdom
import { afterEach, beforeAll, describe, expect, it } from 'vitest'
import { cleanup, render, screen, waitFor, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter, Link, RouterProvider } from 'react-router'
import { useState } from 'react'
import { UnsavedChangesGuard } from './UnsavedChangesGuard'

beforeAll(() => {
  HTMLDialogElement.prototype.showModal = function () { this.open = true }
  HTMLDialogElement.prototype.close = function () { this.open = false }
})
afterEach(cleanup)

function setup(dirty: boolean) {
  const router = createMemoryRouter([
    { path: '/', element: <><UnsavedChangesGuard dirty={dirty} /><Link to="/next">Перейти</Link></> },
    { path: '/next', element: <h1>Следующая страница</h1> },
  ])
  render(<RouterProvider router={router} />)
  return router
}

describe('dirty form protection', () => {
  it('allows clean navigation and does not register unload protection', async () => {
    const router = setup(false)
    const event = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(event)
    expect(event.defaultPrevented).toBe(false)
    await userEvent.setup().click(screen.getByRole('link', { name: 'Перейти' }))
    await screen.findByRole('heading', { name: 'Следующая страница' })
    expect(router.state.location.pathname).toBe('/next')
  })

  it('blocks dirty navigation, focuses stay, preserves route on cancellation and allows confirmation', async () => {
    const user = userEvent.setup()
    const router = setup(true)
    await user.click(screen.getByRole('link', { name: 'Перейти' }))
    await screen.findByRole('dialog', { name: 'Уйти без сохранения?' })
    expect(router.state.location.pathname).toBe('/')
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Остаться' }))
    await user.click(screen.getByRole('button', { name: 'Остаться' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(router.state.location.pathname).toBe('/')
    await user.click(screen.getByRole('link', { name: 'Перейти' }))
    await user.click(await screen.findByRole('button', { name: 'Уйти без сохранения' }))
    await screen.findByRole('heading', { name: 'Следующая страница' })
    expect(router.state.location.pathname).toBe('/next')
  })

  it('blocks browser unload only while dirty and cleans the listener on unmount', () => {
    setup(true)
    const event = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(event)
    expect(event.defaultPrevented).toBe(true)
    cleanup()
    const after = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(after)
    expect(after.defaultPrevented).toBe(false)
  })

  it('also blocks a history back navigation', async () => {
    const router = createMemoryRouter([{ path: '/', element: <h1>Главная</h1> }, { path: '/edit', element: <UnsavedChangesGuard dirty /> }], { initialEntries: ['/', '/edit'], initialIndex: 1 })
    render(<RouterProvider router={router} />)
    await act(async () => { await router.navigate(-1) })
    await screen.findByRole('dialog', { name: 'Уйти без сохранения?' })
    expect(router.state.location.pathname).toBe('/edit')
    await userEvent.setup().click(screen.getByRole('button', { name: 'Уйти без сохранения' }))
    await screen.findByRole('heading', { name: 'Главная' })
  })

  it('continues a blocked navigation when a pending save makes the form clean', async () => {
    let finishSave = () => {}
    function Editable() {
      const [dirty, setDirty] = useState(true)
      finishSave = () => setDirty(false)
      return <><UnsavedChangesGuard dirty={dirty} /><Link to="/next">Перейти</Link></>
    }
    const router = createMemoryRouter([{ path: '/', Component: Editable }, { path: '/next', element: <h1>Сохранено</h1> }])
    render(<RouterProvider router={router} />)
    await userEvent.setup().click(screen.getByRole('link', { name: 'Перейти' }))
    await screen.findByRole('dialog', { name: 'Уйти без сохранения?' })
    await act(async () => { finishSave() })
    await screen.findByRole('heading', { name: 'Сохранено' })
    expect(router.state.location.pathname).toBe('/next')
  })
})
