import { z } from 'zod'
import { shortId } from '../utils/short-id'
export const notificationsSchema = z.object({ notifications: z.array(z.object({
  id: z.string().uuid(), problemId: z.string().uuid(), title: z.string(), actor: z.string(),
  kind: z.enum(['favorite', 'first_accept', 'tester', 'user_editorial']), createdAt: z.string(),
})) })
export type Notification = z.infer<typeof notificationsSchema>['notifications'][number]

export const notificationMessages = { favorite: 'がお気に入りに追加しました', first_accept: 'が初めて正解しました', tester: 'がテスターに参加しました', user_editorial: 'がユーザー解説を書きました' }
// A user editorial opens the editorial tab, where the new post is listed.
export function notificationLink(item: Notification) {
  return item.kind === 'user_editorial' ? `/problems/${shortId(item.problemId)}?view=editorial` : `/problems/${shortId(item.problemId)}`
}
