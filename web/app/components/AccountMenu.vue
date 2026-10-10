<script setup lang="ts">
const { profile, logout } = useAccount()
const route = useRoute()
const open = ref(false)
const root = ref<HTMLElement>()
let timer: ReturnType<typeof setTimeout> | undefined
let dismissed = false
// Touch has no hover, so a tap keeps going straight to my page.
function show(event?: PointerEvent) {
  if (event?.pointerType === 'touch') return
  clearTimeout(timer)
  open.value = true
}
// The delay lets the pointer cross from the avatar to the menu without closing it.
function hide() {
  clearTimeout(timer)
  timer = setTimeout(() => { open.value = false }, 150)
}
// Keyboard focus opens the menu so its links are reachable with Tab; a click or tap focus does not.
function enter(event: FocusEvent) {
  if (!dismissed && event.target instanceof Element && event.target.matches(':focus-visible')) show()
}
function leave(event: FocusEvent) {
  if (!(event.relatedTarget instanceof Node && root.value?.contains(event.relatedTarget))) open.value = false
}
function close() {
  open.value = false
  dismissed = true
  root.value?.querySelector<HTMLElement>('.account-nav')?.focus()
  dismissed = false
}
watch(() => route.fullPath, () => { open.value = false })
onBeforeUnmount(() => clearTimeout(timer))
</script>

<template>
  <div ref="root" class="account-menu" @pointerenter="show" @pointerleave="hide" @focusin="enter" @focusout="leave" @keydown.esc="close">
    <NuxtLink to="/my" class="account-nav" aria-label="マイページ"><UserAvatar :handle="profile?.handle ?? ''" :avatar="profile?.avatar" :size="32" /></NuxtLink>
    <nav v-show="open" class="account-menu-links" aria-label="アカウント">
      <p v-if="profile" class="account-menu-handle">{{ profile.handle }}</p>
      <NuxtLink to="/my"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="8" r="4" /><path d="M4 21a8 8 0 0 1 16 0" /></svg>マイページ</NuxtLink>
      <button type="button" @click="logout"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M15 4h4v16h-4M10 8l-4 4 4 4M6 12h10" /></svg>ログアウト</button>
    </nav>
  </div>
</template>

<style scoped>
.account-menu { position: relative; }
.account-nav { display: flex; align-items: center; min-width: 44px; min-height: 44px; justify-content: center; }
.account-menu-links { position: absolute; right: 0; top: 100%; z-index: 20; display: grid; min-width: max-content; padding: 6px; background: var(--color-paper); border: 1px solid var(--color-line); border-radius: 6px; box-shadow: 0 4px 12px #0001; font-size: .875rem; }
.account-menu-handle { margin: 0; padding: 4px 12px 8px; border-bottom: 1px solid var(--color-line); margin-bottom: 4px; color: var(--color-muted); font-size: .8125rem; overflow-wrap: anywhere; }
.account-menu-links a, .account-menu-links button { display: flex; align-items: center; gap: 8px; min-height: 44px; padding: 0 12px; border: 0; border-radius: 4px; background: none; color: var(--color-ink); font: inherit; text-align: left; text-decoration: none; cursor: pointer; }
.account-menu-links a:hover, .account-menu-links a:focus-visible, .account-menu-links button:hover, .account-menu-links button:focus-visible { background: var(--color-surface); }
</style>
