import { z } from 'zod'
import { shortId } from '../utils/short-id'
const problemNotification = z.object({
  id: z.string().uuid(), problemId: z.string().uuid(), title: z.string(), actor: z.string(),
  kind: z.enum(['favorite', 'first_accept', 'tester', 'user_editorial']), createdAt: z.string(),
})
// A co-setter joining names the contest or, before publication, its draft; the two share the id (ADR 0022).
const contestNotification = z.object({
  id: z.string().uuid(), contestId: z.string().uuid(), title: z.string(), actor: z.string(),
  kind: z.literal('contest_setter'), createdAt: z.string(),
})
export const notificationsSchema = z.object({ notifications: z.array(z.union([problemNotification, contestNotification])) })
export type Notification = z.infer<typeof notificationsSchema>['notifications'][number]

export const notificationMessages = { favorite: 'がお気に入りに追加しました', first_accept: 'が初めて正解しました', tester: 'がテスターに参加しました', user_editorial: 'がユーザー解説を書きました', contest_setter: 'が共同作成者に加わりました' }
export const notificationTitle = (item: Notification) => item.title.trim() || (item.kind === 'contest_setter' ? '無題のコンテスト' : '無題の問題')
// A user editorial opens the editorial tab, where the new post is listed. A new co-setter opens the contest's
// management, where the owner can remove someone who joined through a leaked link.
export function notificationLink(item: Notification) {
  if (item.kind === 'contest_setter') return `/my/contests/${shortId(item.contestId)}?section=manage`
  return item.kind === 'user_editorial' ? `/problems/${shortId(item.problemId)}?view=editorial` : `/problems/${shortId(item.problemId)}`
}
