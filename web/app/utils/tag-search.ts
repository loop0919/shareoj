import { normalizeTag, tagKey, type Tag } from '~~/shared/types/tag'

// Folds case, width, katakana and separators, so 「ﾀﾞｲｸｽﾄﾗ」「だいくすとら」 and 「segment-tree」「Segment Tree」 meet.
function fold(value: string) {
  return normalizeTag(value).toLowerCase()
    .replace(/[ァ-ヶ]/g, ch => String.fromCharCode(ch.charCodeAt(0) - 0x60))
    .replace(/[\s\-_・.]/g, '')
}

// Lower is better: same spelling after folding, then prefix, substring, and letters in order with gaps.
function score(name: string, query: string) {
  if (name === query) return 0
  if (name.startsWith(query)) return 1
  if (name.includes(query)) return 2
  let from = 0
  for (const ch of query) {
    const next = name.indexOf(ch, from)
    if (next < 0) return undefined
    from = next + ch.length
  }
  return 3
}

export type TagMatch = Tag & { exact: boolean }

// Similar tags in use, best match first and then the most used.
export function searchTags(items: Tag[], query: string): TagMatch[] {
  const folded = fold(query)
  if (!folded) return []
  const ranked: { tag: Tag, score: number, uses: number }[] = []
  for (const tag of items) {
    const value = score(fold(tag.name), folded)
    if (value !== undefined) ranked.push({ tag, score: value, uses: tag.problemCount + tag.postCount })
  }
  return ranked.sort((a, b) => a.score - b.score || b.uses - a.uses || a.tag.name.localeCompare(b.tag.name))
    .map(r => ({ ...r.tag, exact: r.score === 0 }))
}

// Reuses the spelling already in use when a tag differs from it only in letter case.
export function canonicalTag(items: Tag[], value: string) {
  const key = tagKey(value)
  return items.find(tag => tagKey(tag.name) === key)?.name ?? normalizeTag(value)
}

export function tagUsage(tag: Tag) {
  return [tag.problemCount ? `問題 ${tag.problemCount}` : '', tag.postCount ? `記事 ${tag.postCount}` : ''].filter(Boolean).join('・')
}
