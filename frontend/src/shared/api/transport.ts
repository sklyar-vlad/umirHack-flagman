/** Передаётся из app через thunk.extraArgument без зависимости shared от store. */
export interface ApiTransport {
  getCsrfToken: () => string | null
}
