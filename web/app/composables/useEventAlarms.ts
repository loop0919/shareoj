import type { BrowserNotificationKind } from '~/composables/useBrowserNotifications'
import { eventAlarms, type Alarm } from '~/utils/event-alarms'

const minute = 60_000
// A time passed by more than this, as on waking from a long sleep, stays quiet.
const late = 5 * minute
// Timers cover the coming day only, well within what setTimeout can wait; later events get theirs on a later read.
const horizon = 24 * 60 * minute
const every = 10 * minute
// A beat later than its minute by more than this means the computer slept. Hidden tabs may hold a beat to the next
// minute, which stays below it.
const slept = 2 * minute
const kinds: BrowserNotificationKind[] = ['contests', 'featured', 'editorials']

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
  // Keys already rung for this user, so an alarm set again or caught up never rings twice.
  const rung = new Set<string>()
  let owner: string | undefined
  // The alarms of the last successful read, set again at once on waking, before the network may be back.
  let latest: Alarm[] = []
  let next: ReturnType<typeof setTimeout> | undefined
  let beat: ReturnType<typeof setTimeout> | undefined
  let last = 0
  let request = 0

  function clear() {
    for (const timer of timers.values()) clearTimeout(timer)
    timers.clear()
  }
  // True when shown.
  function ring(alarm: Alarm) {
    timers.delete(alarm.key)
    if (rung.has(alarm.key)) return false
    rung.add(alarm.key)
    if (Date.now() - alarm.at > late) return false
    show(alarm)
    return true
  }
  // Every wanted alarm gets a fresh timer from the clock, replacing its old one; one whose time just passed rings now.
  function arm(alarms: Alarm[]) {
    clear()
    const now = Date.now()
    for (const alarm of alarms) {
      if (rung.has(alarm.key) || !wants(alarm.kind) || now - alarm.at > late || alarm.at > now + horizon) continue
      if (alarm.at <= now) ring(alarm)
      else timers.set(alarm.key, setTimeout(() => { if (ring(alarm)) void read() }, alarm.at - now))
    }
  }
  function wake() {
    arm(latest)
    void read()
  }
  // Counts minutes without the network to notice a sleep.
  function pulse() {
    const now = Date.now()
    const woke = now - last > minute + slept
    last = now
    beat = setTimeout(pulse, minute)
    if (woke) wake()
  }
  function visible() { if (document.visibilityState === 'visible') wake() }
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
    latest = []
  }
  async function read() {
    clearTimeout(next)
    const current = ++request
    const contests = wants('contests')
    const featured = wants('featured') || wants('editorials')
    if (!user.value || !(contests || featured)) { stop(); return }
    start()
    try {
      const [schedule, page] = await Promise.all([
        contests ? $fetch('/api/my/contest-schedule') : null,
        featured ? $fetch('/api/featured') : null,
      ])
      if (current === request) {
        latest = eventAlarms(schedule?.items ?? [], page)
        arm(latest)
      }
    } catch {
      // The timers already set stay until a later read succeeds.
    } finally {
      if (current === request) next = setTimeout(read, every)
    }
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
