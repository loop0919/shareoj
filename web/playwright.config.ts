import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './tests',
  testIgnore: '**/account/**',
  fullyParallel: true,
  workers: 2,
  forbidOnly: Boolean(process.env.CI),
  retries: 0,
  use: { baseURL: 'http://127.0.0.1:13000', trace: 'retain-on-failure' },
  // Wait for each server's ready line instead of a URL: a URL makes Playwright probe the
  // port first, and on WSL mirrored networking a closed port times out after about two minutes.
  webServer: [
    {
      command: 'node tests/fixtures/public-api.mjs',
      wait: { stdout: /fixture API ready/ },
      timeout: 120_000,
    },
    {
      command: 'node .output/server/index.mjs',
      env: {
        HOST: '127.0.0.1', PORT: '13000',
        NUXT_API_BASE_URL: 'http://127.0.0.1:18080',
        NUXT_PUBLIC_SITE_URL: 'https://judge.example',
      },
      wait: { stdout: /Listening on/ },
    },
    {
      command: 'node tests/fixtures/failing-api.mjs',
      wait: { stdout: /failing API ready/ },
    },
    {
      command: 'node .output/server/index.mjs',
      env: {
        HOST: '127.0.0.1', PORT: '13001',
        NUXT_API_BASE_URL: 'http://127.0.0.1:18081',
        NUXT_PUBLIC_SITE_URL: 'https://judge.example',
      },
      wait: { stdout: /Listening on/ },
    },
  ],
})
