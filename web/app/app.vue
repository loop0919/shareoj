<script setup lang="ts">
const { preference, systemDark } = useTheme()
useHead(() => ({ htmlAttrs: { 'data-theme': preference.value } }))
onMounted(() => {
  const media = window.matchMedia('(prefers-color-scheme: dark)')
  const update = () => { systemDark.value = media.matches }
  update()
  media.addEventListener('change', update)
  onBeforeUnmount(() => media.removeEventListener('change', update))
})
</script>

<template>
  <NuxtLayout>
    <NuxtPage />
  </NuxtLayout>
  <AppToast />
</template>
