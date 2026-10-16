export interface ApiTransport {
  getCsrfToken: () => string | null
  waitForSession?: () => Promise<boolean>
  onUnauthorized?: () => void
  hasSessionAccess?: () => boolean
  getSessionVersion?: () => number
}
