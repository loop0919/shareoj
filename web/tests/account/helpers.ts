import { expect, type Browser, type Page } from '@playwright/test'
import { randomUUID } from 'node:crypto'

// The web server playwright.account.config.ts starts. Writes send it as the Origin header.
export const origin = `http://127.0.0.1:${process.env.OPENOJ_BROWSER_WEB_PORT ?? '13002'}`

// The specs sign in two ways, and each keeps its own: login and user find the email field by its exact label, and
// signIn by a label that contains the text.

// Fills the login form on the open page and sends it. The caller opens the page and checks where it lands.
export async function login(page: Page, name: string) {
  await page.getByLabel('メールアドレス', { exact: true }).fill(`${name}@example.test`)
  await page.getByLabel('パスワード', { exact: true }).fill('test-password')
  await page.getByRole('button', { name: 'ログイン', exact: true }).click()
}

// Opens /login, signs in and waits for /my.
export async function signIn(page: Page, name = 'alice') {
  await page.goto('/login')
  await page.getByLabel('メールアドレス').fill(`${name}@example.test`)
  await page.getByLabel('パスワード', { exact: true }).fill('test-password')
  await page.getByRole('button', { name: 'ログイン', exact: true }).click()
  await expect(page).toHaveURL('/my')
}

// A signed-in person in a browser context of their own.
export async function user(browser: Browser, name: string) {
  const context = await browser.newContext({ baseURL: origin })
  const page = await context.newPage()
  await page.goto('/login')
  await login(page, name)
  await expect(page).toHaveURL('/my')
  return page
}

// Saves a draft problem a contest can take. label goes before the headings, so each spec keeps the text it had.
export async function seedProblem(page: Page, title: string, label = '') {
  const id = randomUUID()
  const response = await page.request.put(`/api/my/problems/${id}`, { headers: { origin }, data: { version: 0, draft: { title, difficulty: 3, markdown: `## ${label}本文\n整数を出力してください。`, editorial: `## ${label}解説\n答えは2です。`, timeLimitMs: '1500', memoryLimitMb: '256', testCases: [{ input: '1', output: '2', isSample: true }] } } })
  expect(response.status()).toBe(200)
  return id
}

// The date input reads Japan time.
export function japanDate(value: number) {
  return new Date(value + 9 * 60 * 60 * 1000).toISOString().slice(0, 16)
}
