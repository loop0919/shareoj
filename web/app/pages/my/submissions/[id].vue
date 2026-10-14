<script setup lang="ts">
definePageMeta({ key: route => String(route.params.id) })
const route = useRoute()
</script>

<template>
  <MySubmissionDetail :id="String(route.params.id)" />
</template>
