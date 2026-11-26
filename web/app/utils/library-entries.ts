import type { z } from 'zod'
import type { AccountSummary } from '~~/shared/types/account-problems'
import type { FolderRef } from '~~/shared/types/folders'
import type { postSummarySchema } from '~~/shared/types/post'
import { refKey } from './folder-tree'
import { problemStatus, type ProblemStatus } from './problem-readiness'

export type PostSummary = z.infer<typeof postSummarySchema>
export type LibraryKind = 'own' | 'testing' | 'post'
export type LibraryStatus = 'preparing' | 'applying' | 'scheduled' | 'published'
// One row of the folder view: an own problem, a tested problem or a post, with the list item it came from.
// createdAt is the update time for an item whose list does not say when it was made. publishedAt is there once the
// item is published, and scheduledAt while a problem waits for its reserved time.
export type LibraryEntry = { key: string, ref: FolderRef, kind: LibraryKind, title: string, updatedAt: string, createdAt: string, publishedAt?: string, scheduledAt?: string, status: LibraryStatus, problem?: AccountSummary, post?: PostSummary }
export type LibrarySort = 'updated' | 'created' | 'published'
export const librarySorts = [{ key: 'updated', label: '更新が新しい順' }, { key: 'created', label: '作成が新しい順' }, { key: 'published', label: '公開が新しい順' }] as const

// The title a row shows and names its buttons by.
export const entryTitle = (entry: LibraryEntry) => entry.title.trim() || (entry.post ? '無題の記事' : '無題の問題')

export const libraryKinds = [{ key: 'own', label: '自分の問題' }, { key: 'testing', label: 'テスト中' }, { key: 'post', label: '記事' }] as const
// The labels the 状態 column shows, so the filter reads the same for authors and testers.
export const problemStatusOptions = [
  { key: 'preparing', label: '準備中' },
  { key: 'applying', label: '応募中' },
  { key: 'scheduled', label: '公開予約中' },
  { key: 'published', label: '公開中' },
] as const
const statusGroups: Record<ProblemStatus['kind'], LibraryStatus> = {
  scheduled: 'scheduled', incomplete: 'preparing', ready: 'preparing', unknown: 'preparing', contest: 'applying', featured: 'applying', published: 'published',
}
export function problemStatusGroup(entry: AccountSummary): LibraryStatus {
  return statusGroups[problemStatus(entry).kind]
}

function problemEntry(problem: AccountSummary, kind: 'own' | 'testing'): LibraryEntry {
  const ref: FolderRef = { kind: 'problem', id: problem.id }
  return { key: refKey(ref), ref, kind, title: problem.title, updatedAt: problem.updatedAt, createdAt: problem.createdAt ?? problem.updatedAt, publishedAt: problem.publishedVersion ? problem.publishedAt : undefined, scheduledAt: problem.scheduledAt, status: problemStatusGroup(problem), problem }
}

function postEntry(post: PostSummary): LibraryEntry {
  const ref: FolderRef = { kind: 'post', id: post.id }
  // A post is either a draft or published, which read as 準備中 and 公開中.
  return { key: refKey(ref), ref, kind: 'post', title: post.title, updatedAt: post.updatedAt, createdAt: post.createdAt ?? post.updatedAt, publishedAt: post.publishedVersion ? post.publishedAt ?? undefined : undefined, status: post.publishedVersion ? 'published' : 'preparing', post }
}

// A copy of the rows, newest first by the time asked for. Times are compared as instants, since their offsets may
// differ; rows of the same instant follow their keys.
// By publication, the published lead from the newest; then come the problems with a reserved time, the soonest first,
// since that one is published next; the rest follow by their last update.
export function sortEntries(entries: LibraryEntry[], by: LibrarySort): LibraryEntry[] {
  const instant = (date: string | undefined) => (date && Date.parse(date)) || 0
  const byKey = (a: LibraryEntry, b: LibraryEntry) => a.key < b.key ? -1 : a.key > b.key ? 1 : 0
  if (by !== 'published') {
    const time = (entry: LibraryEntry) => instant(by === 'created' ? entry.createdAt : entry.updatedAt)
    return [...entries].sort((a, b) => time(b) - time(a) || byKey(a, b))
  }
  const group = (entry: LibraryEntry) => entry.publishedAt ? 0 : entry.scheduledAt ? 1 : 2
  const within = [
    (a: LibraryEntry, b: LibraryEntry) => instant(b.publishedAt) - instant(a.publishedAt),
    (a: LibraryEntry, b: LibraryEntry) => instant(a.scheduledAt) - instant(b.scheduledAt),
    (a: LibraryEntry, b: LibraryEntry) => instant(b.updatedAt) - instant(a.updatedAt),
  ]
  return [...entries].sort((a, b) => group(a) - group(b) || within[group(a)]!(a, b) || byKey(a, b))
}

// The three lists as one, last updated first across kinds.
export function libraryEntries(own: AccountSummary[], testing: AccountSummary[], posts: PostSummary[]): LibraryEntry[] {
  return sortEntries([...own.map(problem => problemEntry(problem, 'own')), ...testing.map(problem => problemEntry(problem, 'testing')), ...posts.map(postEntry)], 'updated')
}
