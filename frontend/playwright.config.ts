import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './tests/browser',
  testMatch: '**/*.pw.ts',
  outputDir: './test-results',
  fullyParallel: false,
  workers: 1,
  use: {
    baseURL: 'http://127.0.0.1:5182',
    headless: true,
    launchOptions: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH } : {},
  },
  webServer: { env: { VITE_ENABLE_MOCKS: 'true', VITE_AUTO_WRITEOFF_READY: 'true', VITE_MOCK_SCENARIO: 'success' }, command: 'npm run dev -- --host 127.0.0.1 --port 5182 --strictPort', url: 'http://127.0.0.1:5182', reuseExistingServer: false },
})
