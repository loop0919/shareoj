import type { ScheduledContest } from '~~/shared/types/contest'
import type { FeaturedPage } from '~~/shared/types/featured'
import { shortId } from '~~/shared/utils/short-id'
import type { BrowserNotice, BrowserNotificationKind } from '~/composables/useBrowserNotifications'

export interface Alarm extends BrowserNotice { at: number, kind: BrowserNotificationKind }

// The browser notices due at known times (ADR 0027). Each key holds its time, so a moved start makes a new alarm.
export function eventAlarms(contests: ScheduledContest[], featured: FeaturedPage | null): Alarm[] {
  const alarms = contests.flatMap((contest): Alarm[] => {
    const page = `/contests/${shortId(contest.id)}`
    return [
      { key: `contest-start:${contest.id}:${contest.startsAt}`, at: Date.parse(contest.startsAt), kind: 'contests', title: 'コンテストが始まりました', body: contest.title, url: `${page}?view=problems` },
      { key: `contest-end:${contest.id}:${contest.endsAt}`, at: Date.parse(contest.endsAt), kind: 'contests', title: 'コンテストが終了しました', body: contest.title, url: `${page}?view=standings` },
    ]
  })
  if (!featured) return alarms
  // A round whose slots are all missing is not released.
  if (featured.nextSlots.some(slot => slot.kind !== 'missing')) {
    alarms.push({ key: `featured-release:${featured.nextAt}`, at: Date.parse(featured.nextAt), kind: 'featured', title: '定期便を出題しました', body: `定期便 vol.${featured.nextNumber}`, url: '/featured' })
  }
  // Revivals show their editorials from the release, so only hidden new problems wait for this notice.
  const round = featured.current
  const hidden = round?.slots.filter(slot => slot.kind === 'new' && slot.editorialHidden) ?? []
  if (round && hidden.length) {
    alarms.push({ key: `featured-editorial:${round.scheduledAt}`, at: Math.min(...hidden.map(slot => Date.parse(slot.revealAt))), kind: 'editorials', title: '定期便の解説を公開しました', body: `定期便 vol.${round.number}`, url: '/featured' })
  }
  return alarms
}
