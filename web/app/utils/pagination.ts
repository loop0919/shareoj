// Page numbers for a paged list, spaced as AtCoder spaces them (ADR 0028): the first and the last page, and those 1, 2,
// 3, 7, 15, 31, … pages away from the current one, each step after 3 being twice the previous plus one.
export function pageNumbers(page: number, last: number) {
  const pages = new Set([1, last, page])
  for (let step = 1; step < last; step = step < 3 ? step + 1 : step * 2 + 1) {
    for (const p of [page - step, page + step]) if (p >= 1 && p <= last) pages.add(p)
  }
  return [...pages].sort((a, b) => a - b)
}

// An empty list still has its one page.
export function lastPage(total: number, size: number) { return Math.max(1, Math.ceil(total / size)) }

// A page past the end shows the last one.
export function clampPage(page: number, last: number) { return Math.min(Math.max(page, 1), last) }

// After the page size changes, the page holding the row that used to top the page.
export function pageAfterResize(page: number, from: number, to: number) { return Math.floor((page - 1) * from / to) + 1 }

// The 1-based positions of the page's first and last rows.
export function pageRange(total: number, page: number, size: number) {
  return { first: (page - 1) * size + 1, last: Math.min(page * size, total) }
}

// The API refuses an offset above this. A page that far out only has to come back empty, so the list can answer with its
// last page; a long run of digits in ?page= would otherwise reach Infinity.
const maxOffset = 1_000_000

// The offset of the requested page's first row, kept to a whole number the API accepts.
export function pageOffset(page: number, size: number) { return Math.min((page - 1) * size, maxOffset) }
