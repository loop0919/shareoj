<script setup lang="ts">
import type { Contest } from '~~/shared/types/contest'
const props = defineProps<{ contest: Contest }>()
const emit = defineEmits<{ joined: [contest: Contest] }>()
const { user } = useAccount()
const route = useRoute()
const saving = ref(false)
const error = ref('')
async function join() {
  if (saving.value) return
  saving.value = true
  error.value = ''
  try {
    const result = await $fetch<Contest>(`/api/my/contests/${props.contest.id}/participation`, { method: 'POST' })
    emit('joined', result)
  } catch {
    error.value = '参加登録できませんでした。ログイン状態・開催期間を確認し、再読み込みしてお試しください。'
  } finally { saving.value = false }
}
</script>

<template>
  <div class="contest-participation">
    <p v-if="user && !contest.official" class="muted">作成者・テスターは公式参加の対象外です。</p>
    <p v-else-if="user && contest.participating" class="participating" role="status">参加済み</p>
    <p v-else-if="contest.status === 'ended'" class="muted">参加受付は終了しました。</p>
    <template v-else>
      <button v-if="user" class="editor-button" :disabled="saving" :aria-busy="saving" @click="join">{{ saving ? '参加登録中…' : '参加する' }}</button>
      <NuxtLink v-else class="editor-button" :to="{ path: '/login', query: { next: route.fullPath } }">ログインして参加する</NuxtLink>
      <p class="muted">参加すると順位表に表示されます。</p>
    </template>
    <p v-if="error" class="notice notice-error" role="alert">{{ error }}</p>
  </div>
</template>

<style scoped>
.contest-participation { margin-block: 24px; }
.contest-participation p { margin: 8px 0; }
.contest-participation .editor-button { min-height: 44px; padding: 8px 20px; color: var(--color-accent); border-color: var(--color-accent); }
.contest-participation button:disabled { opacity: .6; cursor: wait; }
.participating { color: var(--color-accent); font-weight: 600; }
</style>
