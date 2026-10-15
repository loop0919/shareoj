<script setup lang="ts">
import { fullId } from '~~/shared/utils/short-id'
definePageMeta({ key: route => String(route.params.id) })
const route = useRoute()
</script>

<template>
  <MySubmissionDetail :id="fullId(route.params.id)" />
</template>
