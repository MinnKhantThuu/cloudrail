import { defineConfig } from '@playwright/test';
export default defineConfig({
  testDir: './e2e',
  timeout: 120000,
  expect: { timeout: 15000 },
  workers: 1,
  use: { baseURL: 'http://localhost:8080', channel: 'chrome', viewport: { width: 1440, height: 1000 }, screenshot: 'only-on-failure', trace: 'off' },
});
