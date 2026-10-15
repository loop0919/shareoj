<script setup lang="ts">
import { fullId } from '~~/shared/utils/short-id'
definePageMeta({ editorLayout: true, key: route => route.path })
useSeoMeta({ title: 'コンテスト編集 | ShareOJ', robots: 'noindex, nofollow' })
</script>
<template><ContestEditor :contest-id="fullId($route.params.id)" /></template>
