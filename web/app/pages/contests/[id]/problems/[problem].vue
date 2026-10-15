<script setup lang="ts">
import { contestProblemPath } from '~~/shared/types/contest'
import { fullId } from '~~/shared/utils/short-id'
// Older links name a contest problem by its ID; send them to the problem's letter.
definePageMeta({ key: route => route.path })
useResponseHeader('Cache-Control').value = 'no-store'
useResponseHeader('Vary').value = 'Cookie'
const route = useRoute()
const id = fullId(route.params.id)
const pid = fullId(route.params.problem)
const { data: contest, error } = await useContest(id)
const index = contest.value?.problems.findIndex(p => p.id === pid) ?? -1
if (index < 0) throw createError({ statusCode: error.value && error.value.statusCode !== 404 ? 502 : 404, statusMessage: '問題が見つからないか、まだ公開されていません', fatal: true })
// The order is fixed from the start, so only then may clients remember the redirect.
await navigateTo({ path: contestProblemPath(id, index), query: route.query, hash: route.hash }, { replace: true, redirectCode: contest.value!.status === 'scheduled' ? 302 : 301 })
</script>

<template><div /></template>
