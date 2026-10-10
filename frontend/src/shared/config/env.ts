export const env = {
  mockModeEnabled: import.meta.env.DEV && import.meta.env.VITE_ENABLE_MOCKS === 'true',
  mockScenario: import.meta.env.VITE_MOCK_SCENARIO ?? 'success',
} as const
