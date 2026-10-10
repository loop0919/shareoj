// Ranks problems by title and UUID; every space-separated word must hit one of them.
const normalize = (value: string) => value.normalize('NFKC').toLowerCase()

// Lower is better. A substring beats letters found in order with gaps, e.g. 「最短路」 in 「最短経路」.
function titleScore(title: string, word: string) {
  const at = title.indexOf(word)
  if (at >= 0) return at
  let gaps = 0, from = 0
  for (const ch of word) {
    const next = title.indexOf(ch, from)
    if (next < 0) return undefined
    gaps += next - from
    from = next + ch.length
  }
  return 1000 + gaps
}

// Hyphens are optional. Short words only match the start, since a few hex digits appear in most UUIDs.
function idScore(id: string, word: string) {
  const hex = word.replaceAll('-', '')
  if (!hex) return undefined
  const at = id.replaceAll('-', '').indexOf(hex)
  return at === 0 || (at > 0 && hex.length >= 4) ? at : undefined
}

export function searchProblems<T extends { id: string, title: string }>(items: T[], query: string): T[] {
  const words = normalize(query).split(/\s+/).filter(Boolean)
  if (!words.length) return items
  const ranked: { item: T, score: number }[] = []
  for (const item of items) {
    const title = normalize(item.title), id = item.id.toLowerCase()
    let score = 0
    for (const word of words) {
      const best = Math.min(titleScore(title, word) ?? Infinity, idScore(id, word) ?? Infinity)
      if (best === Infinity) { score = Infinity; break }
      score += best
    }
    if (score !== Infinity) ranked.push({ item, score })
  }
  return ranked.sort((a, b) => a.score - b.score).map(r => r.item)
}
