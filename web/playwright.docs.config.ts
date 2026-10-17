import { defineConfig } from '@playwright/test'

// Regenerates the annotated screenshots in public/images/docs from fixture data.
// Run `npm run build` first; the screenshots show the built app, not the dev server.
export default defineConfig({
  testDir: './docs-screenshots',
  workers: 1,
  retries: 0,
  outputDir: 'test-results-docs',
  use: {
    baseURL: 'http://127.0.0.1:13000',
    viewport: { width: 1280, height: 800 },
    deviceScaleFactor: 2,
    colorScheme: 'light',
    locale: 'ja-JP',
    timezoneId: 'Asia/Tokyo',
  },
  webServer: [
    {
      command: 'node tests/fixtures/public-api.mjs',
      wait: { stdout: /fixture API ready/ },
      timeout: 120_000,
    },
    {
      command: 'node docs-screenshots/api.mjs',
      wait: { stdout: /docs screenshot API ready/ },
    },
    {
      command: 'node .output/server/index.mjs',
      env: {
        HOST: '127.0.0.1', PORT: '13000',
        NUXT_API_BASE_URL: 'http://127.0.0.1:18082',
        NUXT_PUBLIC_SITE_URL: 'https://judge.example',
      },
      wait: { stdout: /Listening on/ },
    },
  ],
})
