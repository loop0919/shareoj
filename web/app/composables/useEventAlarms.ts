import type { ScheduledContest } from '~~/shared/types/contest'
import type { FeaturedPage } from '~~/shared/types/featured'
import type { BrowserNotificationKind } from '~/composables/useBrowserNotifications'
import { eventAlarms, type Alarm } from '~/utils/event-alarms'

const minute = 60_000
// A time passed by more than this, as on waking from a long sleep, stays quiet.
const late = 5 * minute
// Timers cover the coming day only, well within what setTimeout can wait; later events get theirs on a later read.
const horizon = 24 * 60 * minute
const every = 10 * minute
// The read after a ring waits a random moment up to this, so the browsers ringing at a contest's start or at 23:00 do
// not all read in the same second.
const spread = 30_000
// A beat later than its minute by more than this means the computer slept. Hidden tabs may hold a beat to the next
// minute, which stays below it.
const slept = 2 * minute
const kinds: BrowserNotificationKind[] = ['contests', 'featured', 'editorials']

// The keys rung in this browser with their times, so neither a reload nor another tab rings them again. It holds one
// user at a time, and each write drops keys too old to catch up.
const rungStorage = 'openoj-rung-alarms'
const kept = 10 * minute
function storedRung(owner: string): Record<string, number> {
  try {
    const saved = JSON.parse(localStorage.getItem(rungStorage) ?? 'null') as { user?: unknown, keys?: unknown } | null
    if (saved?.user === owner && saved.keys && typeof saved.keys === 'object') return saved.keys as Record<string, number>
  } catch { /* Without storage each page remembers only its own. */ }
  return {}
}
function storeRung(owner: string, alarm: Alarm) {
  const since = Date.now() - kept
  const keys = Object.fromEntries(Object.entries(storedRung(owner)).filter(([, at]) => at >= since))
  keys[alarm.key] = alarm.at
  try { localStorage.setItem(rungStorage, JSON.stringify({ user: owner, keys })) }
  catch { /* Without storage each page remembers only its own. */ }
}

// The read of the running useEventAlarms, for pages that change the schedule.
let reread: (() => void) | undefined
// Reads the schedule again now, as after joining a contest. Without the layout's useEventAlarms it does nothing.
export function refreshEventAlarms() { reread?.() }

// Contest starts and ends and 定期便 releases and editorials are known in advance, so the open page notifies at those
// times with one-shot timers, which Chrome barely delays in hidden tabs (ADR 0027). The schedule is read again every 10
// minutes by a chain of timeouts rather than usePolling, which pauses in hidden tabs. The layout calls this once.
// Timers stop while the computer sleeps, so on waking every alarm is set again from the clock, and one whose time
// passed within five minutes rings at once.
export function useEventAlarms() {
  const { user } = useAccount()
  const { wants, show } = useBrowserNotifications()
  const timers = new Map<string, ReturnType<typeof setTimeout>>()
  // Keys this page rang for the user; the stored ones add those of reloads and other tabs.
  const rung = new Set<string>()
  let owner: string | undefined
  // The last answer of each source; one that fails keeps its old answer, so a failing 定期便 leaves the contests alone.
  let schedule: ScheduledContest[] = []
  let featured: FeaturedPage | null = null
  // The alarms of the last read, set again at once on waking, before the network may be back.
  let latest: Alarm[] = []
  let next: ReturnType<typeof setTimeout> | undefined
  let beat: ReturnType<typeof setTimeout> | undefined
  let last = 0
  let request = 0

  function clear() {
    for (const timer of timers.values()) clearTimeout(timer)
    timers.clear()
  }
  // Storage is read each time, so a key another tab rang a moment ago counts.
  function rang(key: string) {
    return rung.has(key) || (!!owner && Object.hasOwn(storedRung(owner), key))
  }
  // True when shown.
  function ring(alarm: Alarm) {
    timers.delete(alarm.key)
    if (Date.now() - alarm.at > late || rang(alarm.key)) return false
    rung.add(alarm.key)
    if (owner) storeRung(owner, alarm)
    show(alarm)
    return true
  }
  // Every wanted alarm gets a fresh timer from the clock, replacing its old one; one whose time just passed rings now.
  function arm(alarms: Alarm[]) {
    clear()
    const now = Date.now()
    for (const alarm of alarms) {
      if (!wants(alarm.kind) || now - alarm.at > late || alarm.at > now + horizon || rang(alarm.key)) continue
      if (alarm.at <= now) ring(alarm)
      else timers.set(alarm.key, setTimeout(() => { if (ring(alarm)) soon() }, alarm.at - now))
    }
  }
  // A ring may bring new times, as a 定期便 release brings the next round's, so the schedule is read again.
  function soon() {
    clearTimeout(next)
    next = setTimeout(read, Math.random() * spread)
  }
  function wake() {
    arm(latest)
    void read()
  }
  // True when the wall clock has moved on much further than a beat since the last look, as across a sleep.
  function asleep() {
    const now = Date.now()
    const gap = now - last
    last = now
    return gap > minute + slept
  }
  // Counts minutes without the network to notice a sleep.
  function pulse() {
    beat = setTimeout(pulse, minute)
    if (asleep()) wake()
  }
  // Showing the page catches up only after a sleep; a tab switch alone keeps the timers and reads nothing.
  function visible() { if (document.visibilityState === 'visible' && asleep()) wake() }
  function start() {
    if (beat !== undefined) return
    last = Date.now()
    beat = setTimeout(pulse, minute)
    document.addEventListener('visibilitychange', visible)
  }
  function stop() {
    clearTimeout(next)
    clearTimeout(beat)
    beat = undefined
    document.removeEventListener('visibilitychange', visible)
    clear()
    schedule = []
    featured = null
    latest = []
  }
  async function read() {
    clearTimeout(next)
    const current = ++request
    const contests = wants('contests')
    const releases = wants('featured') || wants('editorials')
    if (!user.value || !(contests || releases)) { stop(); return }
    start()
    const [mine, page] = await Promise.allSettled([
      contests ? $fetch('/api/my/contest-schedule') : null,
      releases ? $fetch('/api/featured') : null,
    ])
    if (current !== request) return
    // A tab signed out elsewhere stops ringing; another sign-in on this page starts over through the watch below.
    if (mine.status === 'rejected' && (mine.reason as { statusCode?: number }).statusCode === 401) { stop(); return }
    next = setTimeout(read, every)
    if (mine.status === 'fulfilled') schedule = mine.value?.items ?? []
    if (page.status === 'fulfilled') featured = page.value
    latest = eventAlarms(schedule, featured)
    arm(latest)
  }
  onMounted(() => {
    reread = read
    // Another user or setting starts over: the old timers may be someone else's, or of a kind now off. Rung keys stay
    // with their user, so turning a kind off and on repeats nothing.
    watch(() => user.value ? `${user.value.id}:${kinds.filter(kind => wants(kind)).join()}` : '', () => {
      stop()
      if (owner !== user.value?.id) {
        owner = user.value?.id
        rung.clear()
      }
      void read()
    }, { immediate: true })
  })
  onBeforeUnmount(() => {
    ++request
    stop()
    if (reread === read) reread = undefined
  })
}
