import { z } from 'zod'
import { contestProblemPath } from './contest'
import { shortId } from '../utils/short-id'
// A rejudge names the problem, and also its contest and index when the problem is in one.
const problemNotification = z.object({
  id: z.string().uuid(), problemId: z.string().uuid(), contestId: z.string().uuid().optional(), problemIndex: z.number().int().nonnegative().optional(),
  title: z.string(), actor: z.string(),
  kind: z.enum(['favorite', 'first_accept', 'tester', 'user_editorial', 'rejudge']), createdAt: z.string(),
})
// A co-setter joining names the contest or, before publication, its draft; the two share the id (ADR 0022).
// Questions, answers and published results name the contest only.
const contestNotification = z.object({
  id: z.string().uuid(), contestId: z.string().uuid(), title: z.string(), actor: z.string(),
  kind: z.enum(['contest_setter', 'clarification_question', 'clarification_answer', 'clarification_published', 'contest_results']), createdAt: z.string(),
})
export const notificationsSchema = z.object({ notifications: z.array(z.union([problemNotification, contestNotification])) })
export type Notification = z.infer<typeof notificationsSchema>['notifications'][number]

export const notificationMessages = { favorite: 'がお気に入りに追加しました', first_accept: 'が初めて正解しました', tester: 'がテスターに参加しました', user_editorial: 'がユーザー解説を書きました', contest_setter: 'が共同作成者に加わりました',
  clarification_question: 'が質問しました', clarification_answer: 'が質問に回答しました', clarification_published: 'が質問と回答を全体公開しました', contest_results: 'が結果を公開しました',
  rejudge: 'がリジャッジしました' }
export const notificationTitle = (item: Notification) => item.title.trim() || ('problemId' in item ? '無題の問題' : '無題のコンテスト')
// A user editorial opens the editorial tab, where the new post is listed. A new co-setter opens the contest's
// management, where the owner can remove someone who joined through a leaked link.
export function notificationLink(item: Notification) {
  if (item.kind === 'rejudge') {
    if (item.contestId && item.problemIndex !== undefined) return `${contestProblemPath(item.contestId, item.problemIndex)}?view=my-submissions`
    return `/problems/${shortId(item.problemId)}?view=my-submissions`
  }
  if (item.kind === 'contest_setter') return `/my/contests/${shortId(item.contestId)}?section=manage`
  if (item.kind === 'contest_results') return `/contests/${shortId(item.contestId)}?view=standings`
  if (!('problemId' in item)) return `/contests/${shortId(item.contestId)}?view=clarifications`
  return item.kind === 'user_editorial' ? `/problems/${shortId(item.problemId)}?view=editorial` : `/problems/${shortId(item.problemId)}`
}
