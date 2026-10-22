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
  // Chrome for Android's desktop site passes the check below, but its constructor throws; after that the page counts as
  // a phone and stops trying.
  const broken = useState('browser-notifications-broken', () => false)
  onMounted(() => {
    supported.value = !broken.value && typeof Notification === 'function' && !/Android|iPhone|iPad|iPod/i.test(navigator.userAgent)
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
  // Never throws, so a failure leaves the bell and the remaining alarms alone.
  function show(notice: BrowserNotice) {
    let shown: Notification
    try {
      // Many systems cannot show an SVG notification icon.
      shown = new Notification(notice.title, { body: notice.body, tag: notice.key, icon: '/apple-touch-icon.png' })
    } catch (error) {
      if (error instanceof TypeError) {
        broken.value = true
        supported.value = false
        permission.value = 'unsupported'
      }
      return
    }
    shown.onclick = () => {
      window.focus()
      void navigateTo(notice.url)
      shown.close()
    }
  }
  return { settings, supported, permission, enable, wants, show }
}
