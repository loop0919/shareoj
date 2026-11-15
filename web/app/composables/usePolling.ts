// A page nobody has touched for this long polls `idleFactor` times as slowly; the first input brings it up to date.
const idleAfter = 180_000
const idleFactor = 4
// Polls this frequent follow something in progress, such as a judgement, and keep their pace on an idle page.
const progressDelay = 10_000

// One set of listeners serves every poll on the page.
let lastInput = 0
const wakers = new Set<() => void>()
function wakeAll() { for (const wake of [...wakers]) wake() }
function onInput() {
  const idle = Date.now() - lastInput >= idleAfter
  lastInput = Date.now()
  if (idle) wakeAll()
}
function onVisibility() {
  if (document.hidden) return
  lastInput = Date.now()
  wakeAll()
}
const inputEvents = ['pointerdown', 'pointermove', 'keydown', 'wheel', 'touchstart', 'scroll'] as const
function watchInput(wake: () => void) {
  if (!wakers.size) {
    lastInput = Date.now()
    for (const name of inputEvents) window.addEventListener(name, onInput, { capture: true, passive: true })
    document.addEventListener('visibilitychange', onVisibility)
  }
  wakers.add(wake)
}
function unwatchInput(wake: () => void) {
  wakers.delete(wake)
  if (wakers.size) return
  for (const name of inputEvents) window.removeEventListener(name, onInput, { capture: true })
  document.removeEventListener('visibilitychange', onVisibility)
}

interface PollingOptions {
  enabled?: () => boolean
  // A hidden tab skips the task unless this says it is still wanted (e.g. for browser notifications).
  whileHidden?: () => boolean
  // An idle page slows down unless this says the usual pace is still wanted (e.g. standings people only watch).
  whileIdle?: () => boolean
}

// Schedule after completion so slow requests never overlap with the next tick.
export function usePolling(task: () => Promise<unknown>, delay: number, options: PollingOptions = {}) {
  const { enabled = () => true, whileHidden = () => false, whileIdle = () => delay < progressDelay } = options
  let timer: ReturnType<typeof setTimeout> | undefined
  let disposed = false
  let running = false
  let lastRun = 0
  let mounted = false
  const wait = () => Date.now() - lastInput < idleAfter || whileIdle() ? delay : delay * idleFactor
  async function tick() {
    running = true
    try {
      if ((!document.hidden || whileHidden()) && enabled()) {
        lastRun = Date.now()
        await task()
      }
    } finally {
      running = false
      if (!disposed) timer = setTimeout(tick, wait())
    }
  }
  // The page is in use again: run what is overdue now, and bring a slowed timer back to the usual pace.
  function wake() {
    if (running || disposed) return
    clearTimeout(timer)
    const left = lastRun + delay - Date.now()
    if (left > 0) timer = setTimeout(tick, left)
    else void tick()
  }
  onMounted(() => {
    mounted = true
    watchInput(wake)
    lastRun = Date.now()
    timer = setTimeout(tick, delay)
  })
  onScopeDispose(() => {
    disposed = true
    clearTimeout(timer)
    // The server never mounts, and has no window to stop listening on.
    if (mounted) unwatchInput(wake)
  })
}
