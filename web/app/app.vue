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
// A closed route's page never renders in streaming mode, whatever the error state: the outlet shows the closed screen in
// its place, so neither the page's setup nor its reads ever run.
const closedPage = useClosedPage()
</script>

<template>
  <NuxtLayout>
    <StreamingClosed v-if="closedPage" />
    <NuxtPage v-else />
  </NuxtLayout>
  <AppToast />
</template>
