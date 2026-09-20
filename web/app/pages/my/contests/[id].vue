<script setup lang="ts">
definePageMeta({ editorLayout: true, key: route => route.path })
useSeoMeta({ title: 'コンテスト編集 | ShareOJ', robots: 'noindex, nofollow' })
</script>
<template><ContestEditor :contest-id="String($route.params.id)" /></template>
