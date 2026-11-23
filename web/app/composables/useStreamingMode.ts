import { streamingCookie } from '~~/shared/utils/streaming-mode'

const channelName = 'openoj-streaming'

// The cookie as the browser holds it right now. useCookie keeps the value it read at load, so it never sees another tab.
function cookieOn() {
  const entry = document.cookie.split('; ').find(part => part.startsWith(`${streamingCookie}=`))
  if (!entry) return false
  try { return decodeURIComponent(entry.slice(streamingCookie.length + 1)) === '1' } catch { return false }
}
function announce() {
  if (typeof BroadcastChannel === 'undefined') return
  const channel = new BroadcastChannel(channelName)
  channel.postMessage('changed')
  channel.close()
}

// Streaming mode lives in a cookie the web server reads too, so the first HTML already matches. Switching it reloads the
// page, which drops whatever staff-only content the page still holds.
export function useStreamingMode() {
  // The value stays a plain 1 both ways: the default codec would write "1" with quotes and read 1 back as a number.
  const saved = useCookie<string | null>(streamingCookie, { default: () => null, decode: decodeURIComponent, encode: value => encodeURIComponent(String(value)), maxAge: 60 * 60 * 24 * 365, sameSite: 'lax', path: '/' })
  const streaming = computed(() => saved.value === '1')
  // The one confirmation dialog, in the layout, reads this.
  const offOpen = useState('streaming-off-open', () => false)
  // True while the cookie differs from the value this tab was loaded with: the tab may still hold staff-only content.
  const stale = useState('streaming-stale', () => false)
  async function turnOn() {
    saved.value = '1'
    // useCookie writes the cookie in a watcher; the others read it when told, so it must be written first.
    await nextTick()
    announce()
    location.reload()
  }
  function askOff() { offOpen.value = true }
  // Only the confirmation's 「切る」 calls this.
  async function turnOff() {
    saved.value = null
    await nextTick()
    announce()
    location.reload()
  }
  return { streaming, turnOn, askOff, turnOff, offOpen, stale }
}

// Registered once per tab, by the layout. Other tabs announce a change; a tab that missed it looks again when it is
// shown, focused or navigated. A tab that turns stale is covered first, then reloaded; if the reload is refused
// (unsaved editor), it stays covered until the cookie returns to the loaded value.
export function useStreamingSync() {
  const { streaming, stale, offOpen } = useStreamingMode()
  // The server's value, carried in the payload, so a reload never counts as a change.
  const loaded = useState('streaming-loaded', () => streaming.value)
  const route = useRoute()
  function check() {
    const next = cookieOn() !== loaded.value
    const wasStale = stale.value
    stale.value = next
    if (next) offOpen.value = false
    if (next && !wasStale) location.reload()
  }
  onMounted(() => {
    const onVisible = () => { if (document.visibilityState === 'visible') check() }
    const channel = typeof BroadcastChannel === 'undefined' ? undefined : new BroadcastChannel(channelName)
    if (channel) channel.onmessage = check
    document.addEventListener('visibilitychange', onVisible)
    window.addEventListener('focus', check)
    const stop = watch(() => route.fullPath, check)
    onBeforeUnmount(() => {
      channel?.close()
      document.removeEventListener('visibilitychange', onVisible)
      window.removeEventListener('focus', check)
      stop()
    })
  })
}
