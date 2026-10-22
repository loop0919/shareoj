import type { Page } from '@playwright/test'
import { expect, test } from './fixtures/account'
import type { Clarification } from '../shared/types/clarification'
import { shortId } from '../shared/utils/short-id'

const id = '88888888-8888-4888-8888-888888888888'
const problemId = '11111111-1111-4111-8111-111111111111'
const qid = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'
const path = `/contests/${shortId(id)}`
const base = `/api/my/contests/${id}/clarifications`
const question: Clarification = {
  id: qid, problemId, question: '制約の確認 <script>alert(1)</script>', author: 'ui_test',
  answer: '', answeredBy: '', answeredAt: null, publishedAt: null,
  createdAt: '2026-10-22T11:00:00Z', version: 1, canManage: false,
}

async function open(page: Page, staff = false) {
  await page.route(`**/api/contests/${id}`, async route => {
    const response = await route.fetch()
    await route.fulfill({ response, json: { ...await response.json(), participating: !staff, official: !staff, staff } })
  })
  await page.goto(`${path}?view=standings`, { waitUntil: 'networkidle' })
  const refreshed = page.waitForResponse(`**/api/contests/${id}`)
  await page.getByRole('button', { name: '今すぐ更新' }).click()
  await refreshed
  await page.getByRole('navigation', { name: 'コンテストメニュー' }).getByRole('link', { name: '質問', exact: true }).click()
}

test('participant asks a private question and sees an escaped answer', async ({ page }) => {
  let items: Clarification[] = []
  const sent: unknown[] = []
  await page.route(`**${base}*`, async route => {
    if (route.request().method() === 'POST') {
      sent.push(route.request().postDataJSON()); items = [{ ...question }]
      return route.fulfill({ status: 204 })
    }
    return route.fulfill({ json: { items, hasMore: false } })
  })
  await open(page)
  await page.getByLabel('対象問題').selectOption(problemId)
  await page.getByLabel('質問（4,000文字以内）', { exact: true }).fill(question.question)
  await page.getByRole('button', { name: '質問を送信' }).click()
  expect(sent).toEqual([{ problemId, question: question.question }])
  const article = page.getByRole('article')
  await expect(article).toContainText(question.question)
  await expect(article).toContainText('非公開 · 未回答')
  await expect(article.getByRole('button')).toHaveCount(0)
  await expect(article.locator('script')).toHaveCount(0)
  items = [{ ...question, answer: '<img src=x onerror=alert(1)>\n回答', answeredBy: 'alice', answeredAt: question.createdAt, version: 2 }]
  await page.getByRole('button', { name: '質問を更新' }).click()
  await expect(article).toContainText(items[0]!.answer)
  await expect(article.locator('img')).toHaveCount(0)
  await page.setViewportSize({ width: 375, height: 900 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})

test('manager keeps conflicted input, answers, and confirms publication', async ({ page }) => {
  let item = { ...question, canManage: true }
  let conflict = true
  const sent: unknown[] = []
  await page.route(`**${base}**`, route => {
    if (route.request().method() === 'PUT') {
      const body = route.request().postDataJSON()
      sent.push(body)
      if (conflict) {
        conflict = false; item = { ...item, answer: '別の担当者の回答', version: 2 }
        return route.fulfill({ status: 409, json: { data: { code: 'clarification_conflict' } } })
      }
      item = { ...item, ...body.answer ? { answer: body.answer, answeredBy: 'alice', answeredAt: question.createdAt } : {}, ...body.publish ? { publishedAt: question.createdAt } : {}, version: item.version + 1 }
      return route.fulfill({ status: 204 })
    }
    return route.fulfill({ json: { items: [item], hasMore: false } })
  })
  await open(page, true)
  await expect(page.getByRole('button', { name: '質問を送信' })).toHaveCount(0)
  await page.getByRole('button', { name: '回答する', exact: true }).click()
  const answer = page.getByLabel('回答（4,000文字以内）', { exact: true })
  await answer.fill('制約は正しいです。')
  await page.getByRole('button', { name: '回答を保存' }).click()
  await expect(page.getByRole('alert')).toContainText('別の担当者が更新しました')
  await expect(answer).toHaveValue('制約は正しいです。')
  await expect(page.getByRole('button', { name: '回答を保存' })).toBeDisabled()
  await expect(page.getByRole('article')).toContainText('別の担当者の回答')
  await page.getByRole('button', { name: '最新の内容を確認して編集を続ける' }).click()
  await page.getByRole('button', { name: '回答を保存' }).click()
  await expect(page.getByRole('article')).toContainText('回答済み')
  page.once('dialog', dialog => dialog.dismiss())
  await page.getByRole('button', { name: '全体公開する' }).click()
  expect(sent).toHaveLength(2)
  page.once('dialog', async dialog => {
    expect(dialog.message()).toContain('ログインしたすべての利用者')
    await dialog.accept()
  })
  await page.getByRole('button', { name: '全体公開する' }).click()
  await expect(page.getByRole('article')).toContainText('全体公開')
  expect(sent[1]).toEqual({ answer: '制約は正しいです。', version: 2 })
  expect(sent[2]).toEqual({ publish: true, version: 3 })
  await expect(page.getByRole('button', { name: '全体公開する' })).toHaveCount(0)
})

test('anonymous page and proxy require login; writes enforce same origin and body size', async ({ page, request }) => {
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: null } }))
  await page.goto(`${path}?view=clarifications`)
  await expect(page.getByRole('region', { name: '質問', exact: true })).toContainText('閲覧にはログインが必要です')
  for (const [method, url] of [['GET', base], ['POST', base], ['PUT', `${base}/${qid}`]]) {
    const response = await request.fetch(url!, { method, data: {} })
    expect(response.status()).toBe(401)
    expect(response.headers()['cache-control']).toBe('no-store')
  }
  expect((await request.get(`/api/contests/${id}/clarifications`)).status()).toBe(404)
  for (const [method, url] of [['POST', base], ['PUT', `${base}/${qid}`]]) {
    expect((await request.fetch(url!, { method, headers: { cookie: 'openoj_access=test', origin: 'https://other.example' }, data: {} })).status()).toBe(403)
    expect((await request.fetch(url!, { method, headers: { cookie: 'openoj_access=test', origin: 'https://judge.example' }, data: { question: 'a'.repeat(33000) } })).status()).toBe(413)
  }
})
