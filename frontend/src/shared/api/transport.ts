/** Передаётся из app через thunk.extraArgument без зависимости shared от store. */
export interface ApiTransport {
  getCsrfToken: () => string | null
  waitForSession?: () => Promise<boolean>
  onUnauthorized?: () => void
  hasSessionAccess?: () => boolean
  getSessionVersion?: () => number
}
