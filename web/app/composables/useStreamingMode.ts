import type { Ref } from 'vue'
import { closedInStreaming, streamingCookie } from '~~/shared/utils/streaming-mode'

const channelName = 'openoj-streaming'
// Set on the root while the tab may hold staff-only content; the stylesheet hides the app root under it.
export const coveredMark = 'data-streaming-covered'

// The cookie as the browser holds it right now. useCookie follows other tabs through cookieStore where the browser has it,
// and otherwise only their writes through useCookie, each once its event has arrived; reading document.cookie depends on
// neither that API nor its timing.
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

// One cookie ref per app, made in the app's own scope: a ref per call would leave a listener, a watcher and a timer behind
// at every navigation, and a ref made in a component's scope would stop following the cookie when the component unmounts.
const savedCookies = new WeakMap<object, Ref<string | undefined>>()
function savedCookie() {
  const app = useNuxtApp()
  let saved = savedCookies.get(app)
  if (!saved) {
    // The value stays a plain 1 both ways: the default codec would write "1" with quotes and read 1 back as a number.
    // No default: an absent cookie stays absent instead of being deleted again by every page load.
    saved = app._scope.run(() => useCookie<string | undefined>(streamingCookie, { decode: decodeURIComponent, encode: value => encodeURIComponent(String(value)), maxAge: 60 * 60 * 24 * 365, sameSite: 'lax', path: '/' }))!
    savedCookies.set(app, saved)
  }
  return saved
}

// Streaming mode lives in a cookie the web server reads too, so the first HTML already matches. Switching it reloads the
// page, which drops whatever staff-only content the page still holds.
export function useStreamingMode() {
  const saved = savedCookie()
  const streaming = computed(() => saved.value === '1')
  // The mode this document was rendered in: the server's value, carried in the payload, so a reload never counts as a change.
  const loaded = useState('streaming-loaded', () => streaming.value)
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
    saved.value = undefined
    await nextTick()
    announce()
    location.reload()
  }
  return { streaming, loaded, turnOn, askOff, turnOff, offOpen, stale }
}

// Whether the page outlet must show the closed screen instead of the page. It follows the route the router stands on, not
// the error state: after a direct load the router already stands on the closed route, and the error that shows the closed
// screen is cleared by any later move, even one that is cancelled or goes nowhere. The matched record names the route,
// because Nuxt clears the name of the route a direct load starts on. It follows the mode the document was rendered in,
// so a tab that another tab switched keeps its page under the cover instead of losing unsaved edits.
export function useClosedPage() {
  const { loaded } = useStreamingMode()
  const router = useRouter()
  return computed(() => loaded.value && closedInStreaming(String(router.currentRoute.value.matched.at(-1)?.name ?? '')))
}

// Registered once per tab, by the layout. Other tabs announce a change; a tab that missed it looks again when it loads,
// when it is restored from the back/forward cache, and when it is shown, focused or navigated. A tab that turns stale is
// covered first, then reloaded; if the reload is refused (unsaved editor), it stays covered until the cookie returns to
// the loaded value.
export function useStreamingSync() {
  const { stale, offOpen, loaded } = useStreamingMode()
  const route = useRoute()
  function check() {
    const next = cookieOn() !== loaded.value
    const wasStale = stale.value
    stale.value = next
    if (next) offOpen.value = false
    if (next && !wasStale) {
      // A browser may open the leave prompt inside reload(), before the cover renders, so the content is hidden first.
      document.documentElement.setAttribute(coveredMark, '')
      location.reload()
    }
  }
  onMounted(() => {
    const onVisible = () => { if (document.visibilityState === 'visible') check() }
    const channel = typeof BroadcastChannel === 'undefined' ? undefined : new BroadcastChannel(channelName)
    if (channel) channel.onmessage = check
    document.addEventListener('visibilitychange', onVisible)
    window.addEventListener('focus', check)
    window.addEventListener('pageshow', check)
    const stop = watch(() => route.fullPath, check)
    onBeforeUnmount(() => {
      channel?.close()
      document.removeEventListener('visibilitychange', onVisible)
      window.removeEventListener('focus', check)
      window.removeEventListener('pageshow', check)
      stop()
    })
    // The cookie may have changed between the server's render and this mount.
    check()
  })
}
