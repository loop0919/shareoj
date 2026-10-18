<script setup lang="ts">
import { notificationLink, notificationMessages, notificationTitle, type Notification } from '~~/shared/types/notification'
const { user } = useAccount()
const route = useRoute()
const menu = ref<HTMLDetailsElement>()
const items = ref<Notification[]>([])
const unread = ref(0)
const error = ref('')
const opening = ref(false)
let request = 0
async function refresh() {
  if (opening.value || menu.value?.open) return
  const current = ++request
  const owner = user.value?.id
  if (!owner) { items.value = []; unread.value = 0; return }
  try {
    const result = await $fetch('/api/my/notifications')
    if (current === request && user.value?.id === owner) {
      items.value = result.notifications
      unread.value = items.value.length
      error.value = ''
    }
  } catch { if (current === request) error.value = '通知を取得できませんでした。' }
}
function close() { if (menu.value) menu.value.open = false }
function outside(event: MouseEvent) { if (event.target instanceof Node && !menu.value?.contains(event.target)) close() }
async function open() {
  if (opening.value) return
  const current = ++request
  const owner = user.value?.id
  const previousUnread = unread.value
  unread.value = 0
  opening.value = true
  error.value = ''
  try {
    const result = await $fetch('/api/my/notifications/read', { method: 'POST' })
    if (current === request && user.value?.id === owner) items.value = result.notifications
  } catch {
    if (current === request && user.value?.id === owner) {
      unread.value = previousUnread
      error.value = '通知を既読にできませんでした。もう一度お試しください。'
    }
  } finally {
    opening.value = false
    if (!menu.value?.open) void refresh()
  }
}
function toggle() {
  if (menu.value?.open) void open()
  else if (!opening.value) void refresh()
}
onMounted(() => { void refresh(); document.addEventListener('click', outside) })
onBeforeUnmount(() => { ++request; document.removeEventListener('click', outside) })
watch(() => user.value?.id, () => { ++request; items.value = []; unread.value = 0; close(); void refresh() })
watch(() => route.fullPath, () => { close(); void refresh() })
usePolling(refresh, 30_000, () => !!user.value)
</script>

<template>
  <details ref="menu" class="notification-menu" @toggle="toggle" @keydown.esc.prevent="close(); menu?.querySelector('summary')?.focus()">
    <summary :aria-label="`通知（未読${unread}件）`" title="通知">
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9M10 21h4" /></svg>
      <span v-if="unread" class="badge" aria-hidden="true">{{ unread > 99 ? '99+' : unread }}</span>
    </summary>
    <div class="notification-panel">
      <strong>通知</strong>
      <p v-if="error" role="alert">{{ error }} <button type="button" @click="open">再試行</button></p>
      <p v-else-if="!opening && !items.length">未読の通知はありません。</p>
      <ul v-if="items.length">
        <li v-for="item in items" :key="item.id">
          <NuxtLink :to="notificationLink(item)" @click="close">
            <span class="notification-title">{{ notificationTitle(item) }}</span>
            <span>{{ item.actor }}{{ notificationMessages[item.kind] }}</span>
            <time :datetime="item.createdAt">{{ new Date(item.createdAt).toLocaleString('ja-JP') }}</time>
          </NuxtLink>
        </li>
      </ul>
      <NuxtLink class="events-link" to="/my?tab=events" @click="close">イベントをすべて見る</NuxtLink>
    </div>
  </details>
</template>

<style scoped>
summary { position: relative; display: flex; align-items: center; justify-content: center; min-width: 44px; min-height: 44px; cursor: pointer; list-style: none; color: var(--color-accent); }
summary::-webkit-details-marker { display: none; }
.badge { position: absolute; top: 0; right: 0; border-radius: 12px; padding: 1px 5px; background: var(--color-accent); color: var(--color-paper); font-size: .7rem; }
.notification-panel { position: absolute; right: 0; top: 100%; z-index: 25; width: min(360px, calc(100vw - 32px)); max-height: 65vh; overflow-y: auto; padding: 16px; border: 1px solid var(--color-line); border-radius: 6px; background: var(--color-paper); box-shadow: 0 4px 12px #0001; font-size: .875rem; }
ul { list-style: none; padding: 0; margin: 12px 0 0; }
li + li { border-top: 1px solid var(--color-line); }
li a { display: grid; gap: 6px; width: 100%; padding: 12px 4px; text-align: left; background: transparent; border: 0; color: var(--color-ink); cursor: pointer; overflow-wrap: anywhere; text-decoration: none; white-space: normal; }
li a:hover, li a:focus-visible { background: var(--color-surface); }
.events-link { margin-top: 12px; font-size: .875rem; }
.notification-title { font-weight: 600; }
time { font-size: .75rem; color: var(--color-muted); }
</style>
