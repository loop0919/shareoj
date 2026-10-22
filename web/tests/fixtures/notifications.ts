import type { Page } from '@playwright/test'

// Replaces window.Notification before the page loads. requestPermission answers `answer`; every notification created
// is recorded in window.__shown ({ title, body, tag }) and window.__instances, so a test can call its onclick.
export async function fakeNotifications(page: Page, { permission, answer }: { permission: NotificationPermission, answer: NotificationPermission }) {
  await page.addInitScript(({ permission, answer }) => {
    const w = window as unknown as Record<string, unknown>
    w.__shown = []
    w.__instances = []
    // The answer survives a reload, as a real browser's permission does.
    const kept = localStorage.getItem('__notification-permission') as NotificationPermission | null
    class FakeNotification {
      static permission = kept ?? permission
      static requestPermission() {
        FakeNotification.permission = answer
        localStorage.setItem('__notification-permission', answer)
        return Promise.resolve(answer)
      }
      onclick: (() => void) | null = null
      close() {}
      constructor(title: string, options: { body?: string, tag?: string } = {}) {
        (w.__shown as unknown[]).push({ title, body: options.body, tag: options.tag });
        (w.__instances as unknown[]).push(this)
      }
    }
    w.Notification = FakeNotification
  }, { permission, answer })
}

export function shown(page: Page) {
  return page.evaluate(() => (window as unknown as { __shown: { title: string, body?: string, tag?: string }[] }).__shown)
}
