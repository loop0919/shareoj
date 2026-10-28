<script setup lang="ts">
import { deletedHandle } from '~~/shared/types/profile'
const props = defineProps<{ handle?: string }>()
const { profile } = useAccount()
const { data: publicProfile } = useAsyncData(
  () => `user-link:${props.handle ?? ''}`,
  async () => props.handle && !deletedHandle.test(props.handle)
    ? $fetch(`/api/users/${encodeURIComponent(props.handle)}`, { retry: 0 })
    : { handle: '', avatar: '' },
  { server: false, dedupe: 'defer' },
)
const avatar = computed(() => profile.value?.handle === props.handle
  ? profile.value?.avatar
  : publicProfile.value?.handle === props.handle ? publicProfile.value?.avatar : '')
</script>
<template>
  <span v-if="handle && deletedHandle.test(handle)" class="deleted-user">退会したユーザー</span>
  <NuxtLink v-else-if="handle" class="user-link" :to="`/users/${encodeURIComponent(handle)}`"><UserAvatar :handle="handle" :avatar="avatar" :size="20" /><span>{{ handle }}</span></NuxtLink>
  <span v-else>—</span>
</template>
<style scoped>
.user-link { display: inline-flex; align-items: center; gap: .35em; vertical-align: middle; white-space: nowrap; }
</style>
