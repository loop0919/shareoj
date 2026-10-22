import type { BrowserNotificationKind } from '~/composables/useBrowserNotifications'
import { eventAlarms, type Alarm } from '~/utils/event-alarms'

const minute = 60_000
// A timer that fires later than this after its time, as on waking from sleep, stays quiet.
const late = 5 * minute
// Timers cover the coming day only, well within what setTimeout can wait; later events get theirs on a later read.
const horizon = 24 * 60 * minute
const every = 10 * minute
const kinds: BrowserNotificationKind[] = ['contests', 'featured', 'editorials']

// The read of the running useEventAlarms, for pages that change the schedule.
let reread: (() => void) | undefined
// Reads the schedule again now, as after joining a contest. Without the layout's useEventAlarms it does nothing.
export function refreshEventAlarms() { reread?.() }

// Contest starts and ends and 定期便 releases and editorials are known in advance, so the open page notifies at those
// times with one-shot timers, which Chrome barely delays in hidden tabs (ADR 0027). The schedule is read again every 10
// minutes by a chain of timeouts rather than usePolling, which pauses in hidden tabs. The layout calls this once.
export function useEventAlarms() {
  const { user } = useAccount()
  const { wants, show } = useBrowserNotifications()
  const timers = new Map<string, ReturnType<typeof setTimeout>>()
  let next: ReturnType<typeof setTimeout> | undefined
  let request = 0

  function clear() {
    for (const timer of timers.values()) clearTimeout(timer)
    timers.clear()
  }
  function ring(alarm: Alarm) {
    timers.delete(alarm.key)
    if (Date.now() - alarm.at > late) return
    show(alarm)
    void read()
  }
  // Keys still due keep their timers; a moved time comes with a new key.
  function arm(alarms: Alarm[]) {
    const now = Date.now()
    const due = new Map(alarms.filter(alarm => now < alarm.at && alarm.at <= now + horizon && wants(alarm.kind)).map(alarm => [alarm.key, alarm]))
    for (const [key, timer] of timers) {
      if (!due.has(key)) { clearTimeout(timer); timers.delete(key) }
    }
    for (const [key, alarm] of due) {
      if (!timers.has(key)) timers.set(key, setTimeout(() => ring(alarm), alarm.at - now))
    }
  }
  async function read() {
    clearTimeout(next)
    const current = ++request
    const contests = wants('contests')
    const featured = wants('featured') || wants('editorials')
    if (!user.value || !(contests || featured)) { clear(); return }
    try {
      const [schedule, page] = await Promise.all([
        contests ? $fetch('/api/my/contest-schedule') : null,
        featured ? $fetch('/api/featured') : null,
      ])
      if (current === request) arm(eventAlarms(schedule?.items ?? [], page))
    } catch {
      // The timers already set stay until a later read succeeds.
    } finally {
      if (current === request) next = setTimeout(read, every)
    }
  }
  onMounted(() => {
    reread = read
    // Another user or setting starts over: the old timers may be someone else's, or of a kind now off.
    watch(() => user.value ? `${user.value.id}:${kinds.filter(kind => wants(kind)).join()}` : '', () => { clear(); void read() }, { immediate: true })
  })
  onBeforeUnmount(() => {
    ++request
    clearTimeout(next)
    clear()
    if (reread === read) reread = undefined
  })
}
