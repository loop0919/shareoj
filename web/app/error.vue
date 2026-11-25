<script setup lang="ts">
import type { NuxtError } from '#app'
import { streamingCode } from '~~/shared/utils/streaming-mode'

const props = defineProps<{ error: NuxtError<{ code?: string, data?: { code?: string } }> }>()
const notFound = computed(() => props.error.statusCode === 404)
const closed = computed(() => props.error.statusCode === 403 && (props.error.data?.data?.code === streamingCode || props.error.data?.code === streamingCode))
const title = computed(() => closed.value ? '配信モード中は表示できません' : notFound.value ? 'ページが見つかりません' : 'ページを表示できませんでした')
useSeoMeta({ title: () => `${title.value} | ShareOJ`, robots: 'noindex, nofollow' })
</script>

<template>
  <NuxtLayout>
    <StreamingClosed v-if="closed" />
    <div v-else class="error-page">
      <p class="eyebrow">{{ error.statusCode }}</p>
      <h1>{{ title }}</h1>
      <p>{{ notFound ? 'URL を確認するか、問題から選び直してください。' : '時間をおいて、もう一度アクセスしてください。' }}</p>
      <a href="/problems" class="return-link">問題へ戻る <span aria-hidden="true">→</span></a>
    </div>
  </NuxtLayout>
</template>
