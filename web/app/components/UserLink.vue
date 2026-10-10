<script setup lang="ts">
import { deletedHandle } from '~~/shared/types/profile'
defineProps<{ handle?: string }>()
const { profile } = useAccount()
</script>
<template>
  <span v-if="handle && deletedHandle.test(handle)" class="deleted-user">退会したユーザー</span>
  <NuxtLink v-else-if="handle" :to="profile?.handle === handle ? '/my' : `/users/${encodeURIComponent(handle)}`">{{ handle }}</NuxtLink>
  <span v-else>—</span>
</template>
