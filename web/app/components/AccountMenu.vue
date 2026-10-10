<script setup lang="ts">
const { profile, logout } = useAccount()
const route = useRoute()
const open = ref(false)
const root = ref<HTMLElement>()
function close(refocus = false) {
  open.value = false
  if (refocus) root.value?.querySelector<HTMLElement>('.account-nav')?.focus()
}
function outside(event: PointerEvent) {
  if (open.value && event.target instanceof Node && !root.value?.contains(event.target)) close()
}
// Tabbing past the last item closes the menu, like a click elsewhere.
function leave(event: FocusEvent) {
  if (!(event.relatedTarget instanceof Node && root.value?.contains(event.relatedTarget))) close()
}
async function signOut() {
  close()
  await logout()
}
watch(() => route.fullPath, () => close())
onMounted(() => document.addEventListener('pointerdown', outside))
onBeforeUnmount(() => document.removeEventListener('pointerdown', outside))
</script>

<template>
  <div ref="root" class="account-menu" @focusout="leave" @keydown.esc="close(true)">
    <button type="button" class="account-nav" aria-label="アカウントメニュー" :aria-expanded="open" aria-controls="account-menu-links" @click="open = !open"><UserAvatar :handle="profile?.handle ?? ''" :avatar="profile?.avatar" :size="32" /></button>
    <nav v-show="open" id="account-menu-links" class="account-menu-links" aria-label="アカウント">
      <p v-if="profile" class="account-menu-handle">{{ profile.handle }}</p>
      <NuxtLink to="/my"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="8" r="4" /><path d="M4 21a8 8 0 0 1 16 0" /></svg>マイページ</NuxtLink>
      <NuxtLink to="/my/settings"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="3" /><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1Z" /></svg>設定</NuxtLink>
      <button type="button" class="account-logout" @click="signOut"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M15 4h4v16h-4M10 8l-4 4 4 4M6 12h10" /></svg>ログアウト</button>
    </nav>
  </div>
</template>

<style scoped>
.account-menu { position: relative; }
.account-nav { display: flex; align-items: center; justify-content: center; min-width: 44px; min-height: 44px; padding: 0; border: 0; border-radius: 50%; background: none; cursor: pointer; }
.account-nav:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 2px; }
.account-menu-links { position: absolute; right: 0; top: 100%; z-index: 20; display: grid; min-width: max-content; padding: 6px; background: var(--color-paper); border: 1px solid var(--color-line); border-radius: 6px; box-shadow: 0 4px 12px #0001; font-size: .875rem; }
.account-menu-handle { margin: 0; padding: 4px 12px 8px; border-bottom: 1px solid var(--color-line); margin-bottom: 4px; color: var(--color-muted); font-size: .8125rem; overflow-wrap: anywhere; }
.account-menu-links a, .account-menu-links button { display: flex; align-items: center; gap: 8px; min-height: 44px; padding: 0 12px; border: 0; border-radius: 4px; background: none; color: var(--color-ink); font: inherit; text-align: left; text-decoration: none; cursor: pointer; }
.account-menu-links .account-logout { color: var(--color-error); }
.account-menu-links a:hover, .account-menu-links a:focus-visible, .account-menu-links button:hover, .account-menu-links button:focus-visible { background: var(--color-surface); }
</style>
