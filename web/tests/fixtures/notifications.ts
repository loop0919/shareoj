import type { Page } from '@playwright/test'

// Replaces window.Notification before the page loads. requestPermission answers `answer`; every notification created
// is recorded in window.__shown ({ title, body, tag }) and window.__instances, so a test can call its onclick.
// With `throws`, the constructor fails as on Chrome for Android's desktop site and counts each try in window.__attempts.
export async function fakeNotifications(page: Page, { permission, answer, throws = false }: { permission: NotificationPermission, answer: NotificationPermission, throws?: boolean }) {
  await page.addInitScript(({ permission, answer, throws }) => {
    const w = window as unknown as Record<string, unknown>
    w.__shown = []
    w.__instances = []
    w.__attempts = 0
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
        w.__attempts = (w.__attempts as number) + 1
        if (throws) throw new TypeError('Failed to construct \'Notification\': Illegal constructor. Use ServiceWorkerRegistration.showNotification() instead.');
        (w.__shown as unknown[]).push({ title, body: options.body, tag: options.tag });
        (w.__instances as unknown[]).push(this)
      }
    }
    w.Notification = FakeNotification
  }, { permission, answer, throws })
}

export function shown(page: Page) {
  return page.evaluate(() => (window as unknown as { __shown: { title: string, body?: string, tag?: string }[] }).__shown)
}

export function attempts(page: Page) {
  return page.evaluate(() => (window as unknown as { __attempts: number }).__attempts)
}
