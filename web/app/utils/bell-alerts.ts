import { notificationLink, notificationTitle, type Notification } from '~~/shared/types/notification'
import type { BrowserNotice } from '~/composables/useBrowserNotifications'

const titles: Partial<Record<Notification['kind'], string>> = {
  clarification_question: '質問が届きました',
  clarification_answer: '質問に回答がありました',
  clarification_published: '質問・回答が全体公開されました',
}

// The browser notice for a bell item found on a poll, or null for kinds that only the bell shows.
export function bellAlert(item: Notification): BrowserNotice | null {
  const title = titles[item.kind]
  if (!title) return null
  return { key: `notification:${item.id}`, title, body: notificationTitle(item), url: notificationLink(item) }
}
