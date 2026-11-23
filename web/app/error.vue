<script setup lang="ts">
import type { NuxtError } from '#app'
import { streamingCode } from '~~/shared/utils/streaming-mode'

const props = defineProps<{ error: NuxtError<{ code?: string, data?: { code?: string } }> }>()
const { askOff } = useStreamingMode()
const router = useRouter()
// After a failed in-site move the router already stands on the page it left while the address bar shows the closed one, so the move home is forced.
async function goHome() {
  await clearError()
  await router.push({ path: '/', force: true })
}
const notFound = computed(() => props.error.statusCode === 404)
const closed = computed(() => props.error.statusCode === 403 && (props.error.data?.data?.code === streamingCode || props.error.data?.code === streamingCode))
const title = computed(() => closed.value ? '配信モード中は表示できません' : notFound.value ? 'ページが見つかりません' : 'ページを表示できませんでした')
useSeoMeta({ title: () => `${title.value} | ShareOJ`, robots: 'noindex, nofollow' })
</script>

<template>
  <NuxtLayout>
    <div v-if="closed" class="error-page">
      <h1>{{ title }}</h1>
      <p>このページは、配信に映らないように閉じています。</p>
      <p class="closed-actions">
        <NuxtLink to="/" class="return-link" @click.prevent="goHome">ホームへ戻る</NuxtLink>
        <button type="button" class="return-link" @click="askOff">配信モードを切る</button>
      </p>
    </div>
    <div v-else class="error-page">
      <p class="eyebrow">{{ error.statusCode }}</p>
      <h1>{{ title }}</h1>
      <p>{{ notFound ? 'URL を確認するか、問題から選び直してください。' : '時間をおいて、もう一度アクセスしてください。' }}</p>
      <a href="/problems" class="return-link">問題へ戻る <span aria-hidden="true">→</span></a>
    </div>
  </NuxtLayout>
</template>

<style scoped>
.closed-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 16px; }
button.return-link { padding: 0; border: 0; background: none; font: inherit; cursor: pointer; }
</style>
