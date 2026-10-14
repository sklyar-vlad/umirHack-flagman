import { expect, type Locator, type Page } from '@playwright/test'

/** A background read must retain its loaded content and finish before the next scenario. */
export async function backgroundRefresh(page: Page, scenario: string, pathname: string, label: string, retained: Locator, button?: Locator) {
  const response = page.waitForResponse((response) => new URL(response.url()).pathname === pathname && response.request().method() === 'GET')
  // Keep the actual DOM controls, not just their values: replacing the form loses focus too.
  const controls = await page.locator('main input, main select, main textarea').elementHandles()
  const values = await Promise.all(controls.map((control) => control.evaluate((node) => (node as HTMLInputElement).value)))
  await page.getByLabel('Сценарий API').selectOption(scenario)
  await expect(page.getByRole('status', { name: label })).toHaveCount(0)
  await expect(retained).toBeVisible()
  for (const [index, control] of controls.entries()) {
    expect(await control.evaluate((node) => node.isConnected)).toBe(true)
    expect(await control.evaluate((node) => (node as HTMLInputElement).value)).toBe(values[index])
  }
  if (button) await expect(button).toBeDisabled()
  expect((await response).ok()).toBe(true)
  if (button) await expect(button).toBeEnabled()
  await Promise.all(controls.map((control) => control.dispose()))
}
