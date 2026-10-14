<script setup lang="ts">
definePageMeta({ key: route => route.path })
const route = useRoute()
</script>

<template>
  <MySubmissionDetail :id="String(route.params.submission)" :contest-id="String(route.params.id)" />
</template>
