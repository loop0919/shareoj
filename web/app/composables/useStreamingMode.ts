import { streamingCookie } from '~~/shared/utils/streaming-mode'

// Streaming mode lives in a cookie the web server reads too, so the first HTML already matches. Switching it reloads the
// page, which drops whatever staff-only content the page still holds.
export function useStreamingMode() {
  // The value stays a plain 1 both ways: the default codec would write "1" with quotes and read 1 back as a number.
  const saved = useCookie<string | null>(streamingCookie, { default: () => null, decode: decodeURIComponent, encode: value => encodeURIComponent(String(value)), maxAge: 60 * 60 * 24 * 365, sameSite: 'lax', path: '/' })
  const streaming = computed(() => saved.value === '1')
  // The one confirmation dialog, in the layout, reads this.
  const offOpen = useState('streaming-off-open', () => false)
  function turnOn() {
    saved.value = '1'
    location.reload()
  }
  function askOff() { offOpen.value = true }
  // Only the confirmation's 「切る」 calls this.
  function turnOff() {
    saved.value = null
    location.reload()
  }
  return { streaming, turnOn, askOff, turnOff, offOpen }
}
