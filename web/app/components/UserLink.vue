<script setup lang="ts">
import { deletedHandle } from '~~/shared/types/profile'
const props = defineProps<{ handle?: string }>()
const { profile } = useAccount()
const icon = useUserIcon(() => props.handle)
const avatar = computed(() => profile.value?.handle === props.handle ? profile.value?.avatar : icon.value)
</script>
<template>
  <span v-if="handle && deletedHandle.test(handle)" class="deleted-user">退会したユーザー</span>
  <NuxtLink v-else-if="handle" class="user-link" :to="`/users/${encodeURIComponent(handle)}`"><UserAvatar :handle="handle" :avatar="avatar" :size="20" /><span>{{ handle }}</span></NuxtLink>
  <span v-else>—</span>
</template>
<style scoped>
.user-link { display: inline-flex; align-items: center; gap: .35em; vertical-align: middle; white-space: nowrap; }
</style>
