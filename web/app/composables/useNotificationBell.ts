import type { Notification } from '~~/shared/types/notification'

// The bell's notifications, shared by the layout, which polls them on every page (useNotificationPolling), and
// NotificationBell, which shows them. Editor pages hide the bell, so the alerts for new questions cannot live in it.
export function useNotificationBell() {
  const { user } = useAccount()
  const { wants, show } = useBrowserNotifications()
  const items = useState<Notification[]>('bell-items', () => [])
  const unread = useState('bell-unread', () => 0)
  const error = useState('bell-error', () => '')
  const opening = useState('bell-opening', () => false)
  // The bell's panel is open; a poll then leaves the list being read alone.
  const viewing = useState('bell-viewing', () => false)
  // Each request takes the next number, and only the latest one's answer counts.
  const request = useState('bell-request', () => 0)
  // Ids this page has already seen; null until the first successful fetch, which only fills it.
  const seen = useState<Set<string> | null>('bell-seen', () => null)
  // A 401 means the tab was signed out elsewhere; its alerts stay quiet until the user changes.
  const signedOut = useState('bell-signed-out', () => false)

  function announce(list: Notification[]) {
    if (!seen.value) { seen.value = new Set(list.map(item => item.id)); return }
    for (const item of list) {
      if (seen.value.has(item.id)) continue
      seen.value.add(item.id)
      const alert = bellAlert(item)
      if (alert && !signedOut.value && wants('clarifications')) show(alert)
    }
  }
  async function refresh() {
    if (opening.value || viewing.value) return
    const current = ++request.value
    const owner = user.value?.id
    if (!owner) { items.value = []; unread.value = 0; return }
    try {
      const result = await $fetch('/api/my/notifications')
      if (current === request.value && user.value?.id === owner) {
        // The list goes in before the alerts, so the bell shows it whatever they do.
        items.value = result.notifications
        unread.value = items.value.length
        error.value = ''
        announce(result.notifications)
      }
    } catch (failure) {
      if (current !== request.value) return
      if ((failure as { statusCode?: number }).statusCode === 401) signedOut.value = true
      error.value = '通知を取得できませんでした。'
    }
  }
  async function open() {
    if (opening.value) return
    const current = ++request.value
    const owner = user.value?.id
    const previousUnread = unread.value
    unread.value = 0
    opening.value = true
    error.value = ''
    try {
      const result = await $fetch('/api/my/notifications/read', { method: 'POST' })
      if (current === request.value && user.value?.id === owner) items.value = result.notifications
    } catch {
      if (current === request.value && user.value?.id === owner) {
        unread.value = previousUnread
        error.value = '通知を既読にできませんでした。もう一度お試しください。'
      }
    } finally {
      opening.value = false
      if (!viewing.value) void refresh()
    }
  }
  // Another user starts over: the list and the seen ids were someone else's.
  function reset() {
    ++request.value
    seen.value = null
    signedOut.value = false
    items.value = []
    unread.value = 0
  }
  return { items, unread, error, opening, viewing, refresh, open, reset }
}

// The layout calls this once, so each page polls the bell exactly once, with or without the bell on screen.
export function useNotificationPolling() {
  const { user } = useAccount()
  const route = useRoute()
  const { wants } = useBrowserNotifications()
  const { refresh, reset } = useNotificationBell()
  onMounted(() => { void refresh() })
  watch(() => user.value?.id, () => { reset(); void refresh() })
  watch(() => route.fullPath, () => { void refresh() })
  // Hidden tabs keep polling only for browser notifications of questions.
  usePolling(refresh, 30_000, () => !!user.value, () => wants('clarifications'))
}
