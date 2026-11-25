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
export type LibraryEntry = { key: string, ref: FolderRef, kind: LibraryKind, title: string, updatedAt: string, status: LibraryStatus, problem?: AccountSummary, post?: PostSummary }

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
  return { key: refKey(ref), ref, kind, title: problem.title, updatedAt: problem.updatedAt, status: problemStatusGroup(problem), problem }
}

function postEntry(post: PostSummary): LibraryEntry {
  const ref: FolderRef = { kind: 'post', id: post.id }
  // A post is either a draft or published, which read as 準備中 and 公開中.
  return { key: refKey(ref), ref, kind: 'post', title: post.title, updatedAt: post.updatedAt, status: post.publishedVersion ? 'published' : 'preparing', post }
}

// The three lists as one, newest first across kinds. Times are compared as instants, since their offsets may differ.
export function libraryEntries(own: AccountSummary[], testing: AccountSummary[], posts: PostSummary[]): LibraryEntry[] {
  const time = (entry: LibraryEntry) => Date.parse(entry.updatedAt) || 0
  return [...own.map(problem => problemEntry(problem, 'own')), ...testing.map(problem => problemEntry(problem, 'testing')), ...posts.map(postEntry)]
    .sort((a, b) => time(b) - time(a) || (a.key < b.key ? -1 : a.key > b.key ? 1 : 0))
}
