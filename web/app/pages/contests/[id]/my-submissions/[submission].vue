<script setup lang="ts">
import { fullId } from '~~/shared/utils/short-id'
definePageMeta({ key: route => route.path })
const route = useRoute()
</script>

<template>
  <MySubmissionDetail :id="fullId(route.params.submission)" :contest-id="fullId(route.params.id)" />
</template>
