import type { Standing } from '~~/shared/types/contest'

// A team's row is the viewer's when they are a member; any other row when the handle is theirs.
export function isOwnStanding(row: Standing, handle: string | undefined) {
  if (!handle) return false
  return row.team ? row.team.members.includes(handle) : row.handle === handle
}

// The viewer's row when it lies outside the page (positions start up to, not including, end), and on which side. A tie
// across the page edge is placed by position, not by rank.
export function pinnedStanding(rows: Standing[], start: number, end: number, handle: string | undefined) {
  const index = rows.findIndex(row => isOwnStanding(row, handle))
  if (index < 0 || (index >= start && index < end)) return null
  return { row: rows[index]!, place: index < start ? 'before' as const : 'after' as const }
}
