import { defineConfig } from '@playwright/test'

if (!process.env.TEST_DATABASE_URL) throw new Error('TEST_DATABASE_URL is required')
const apiAddress = process.env.OPENOJ_BROWSER_API_ADDR ?? '127.0.0.1:18082'
const webPort = process.env.OPENOJ_BROWSER_WEB_PORT ?? '13002'
const siteURL = `http://127.0.0.1:${webPort}`

export default defineConfig({
  testDir: './tests/account', outputDir: './test-results-account', workers: 1, retries: 0,
  use: { baseURL: siteURL, trace: 'retain-on-failure' },
  webServer: [
    {
      command: 'go test -v ./internal/httpapi -run ^TestBrowserFixture$ -timeout 0 -count=1', cwd: '../api',
      env: { OPENOJ_BROWSER_TEST: '1', TEST_DATABASE_URL: process.env.TEST_DATABASE_URL, OPENOJ_BROWSER_API_ADDR: apiAddress },
      wait: { stdout: /browser API ready/ }, timeout: 120000,
      gracefulShutdown: { signal: 'SIGTERM', timeout: 10000 },
    },
    {
      command: 'node --import ./tests/fixtures/cognito-token.mjs .output/server/index.mjs',
      env: { HOST: '127.0.0.1', PORT: webPort, NUXT_API_BASE_URL: `http://${apiAddress}`, NUXT_PUBLIC_SITE_URL: siteURL, NUXT_COGNITO_DOMAIN: 'https://example.auth.ap-northeast-1.amazoncognito.com', NUXT_COGNITO_CLIENT_ID: 'browser-client', NUXT_COGNITO_CLIENT_SECRET: 'browser-secret' },
      wait: { stdout: /Listening on/ },
    },
  ],
})
