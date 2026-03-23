import { defineConfig } from '@playwright/test'

const localChromeChannel = process.platform === 'darwin' && !process.env.CI ? 'chrome' : undefined

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['list'], ['html', { outputFolder: 'playwright-report', open: 'never' }]] : 'list',
  outputDir: 'test-results',
  use: {
    baseURL: process.env.CONSOLE_UI_URL || 'http://127.0.0.1:3000',
    channel: localChromeChannel,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
})
