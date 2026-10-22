// Schedule after completion so slow requests never overlap with the next tick.
// A hidden tab skips the task unless `whileHidden` says it is still wanted (e.g. for browser notifications).
export function usePolling(task: () => Promise<unknown>, delay: number, enabled: () => boolean = () => true, whileHidden: () => boolean = () => false) {
  let timer: ReturnType<typeof setTimeout> | undefined
  let disposed = false
  async function tick() {
    try { if ((!document.hidden || whileHidden()) && enabled()) await task() }
    finally { if (!disposed) timer = setTimeout(tick, delay) }
  }
  onMounted(() => { timer = setTimeout(tick, delay) })
  onScopeDispose(() => { disposed = true; clearTimeout(timer) })
}
