import type { Standing, StandingCell } from '~~/shared/types/contest'
import { shortId } from '~~/shared/utils/short-id'

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

// Where a cell's score leads: the submission that set it, or nowhere when the viewer may not read it. all tells that
// the contest's whole list is open to the viewer, as after the end or to the staff. Until then only the own row
// links: a team reads its members' submissions on the shared page, and someone alone reads their own on theirs.
export function standingSubmissionPath(contestId: string, row: Standing, cell: StandingCell, viewer: { all: boolean, handle: string | undefined }) {
  if (!cell.submissionId || !(viewer.all || isOwnStanding(row, viewer.handle))) return ''
  const list = viewer.all || row.team ? 'submissions' : 'my-submissions'
  return `/contests/${shortId(contestId)}/${list}/${shortId(cell.submissionId)}?from=standings`
}
