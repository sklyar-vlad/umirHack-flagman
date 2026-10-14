import { defineConfig } from '@playwright/test'

if (!process.env.FRONTEND_TEST_ORIGIN || !process.env.FRONTEND_TEST_PASSWORD) {
  throw new Error('Run npm run test:production to provision isolated acceptance services')
}

export default defineConfig({
  testDir: './tests/production',
  testMatch: '**/*.pw.ts',
  outputDir: './test-results/production',
  workers: 1,
  fullyParallel: false,
  timeout: 90_000,
  use: {
    baseURL: process.env.FRONTEND_TEST_ORIGIN,
    headless: true,
    // No traces, request attachments or video containing generated credentials.
    launchOptions: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH } : {},
  },
})
