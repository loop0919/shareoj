export type BrowserNotificationKind = 'clarifications' | 'contests' | 'featured' | 'editorials'
export interface BrowserNotificationSettings { enabled: boolean, kinds: Record<BrowserNotificationKind, boolean> }
export interface BrowserNotice { key: string, title: string, body: string, url: string }

// Browser notifications are shown only by open ShareOJ pages (no push, no service worker). The choice lives in a
// cookie like the other display settings. Phones are left to the bell: they cannot show a notification from a page
// reliably, so they count as unsupported.
export function useBrowserNotifications() {
  const settings = useCookie<BrowserNotificationSettings>('openoj-browser-notifications', {
    default: () => ({ enabled: false, kinds: { clarifications: true, contests: true, featured: true, editorials: true } }),
    maxAge: 60 * 60 * 24 * 365,
    sameSite: 'lax',
    path: '/',
  })
  const supported = useState('browser-notifications-supported', () => false)
  const permission = useState<NotificationPermission | 'unsupported'>('browser-notifications-permission', () => 'unsupported')
  onMounted(() => {
    supported.value = typeof Notification === 'function' && !/Android|iPhone|iPad|iPod/i.test(navigator.userAgent)
    permission.value = supported.value ? Notification.permission : 'unsupported'
  })

  async function enable() {
    if (!supported.value) return
    permission.value = await Notification.requestPermission()
    if (permission.value === 'granted') settings.value = { ...settings.value, enabled: true }
  }
  function wants(kind: BrowserNotificationKind) {
    return supported.value && permission.value === 'granted' && settings.value.enabled && settings.value.kinds[kind]
  }
  function show(notice: BrowserNotice) {
    const shown = new Notification(notice.title, { body: notice.body, tag: notice.key, icon: '/favicon.svg' })
    shown.onclick = () => {
      window.focus()
      void navigateTo(notice.url)
      shown.close()
    }
  }
  return { settings, supported, permission, enable, wants, show }
}
