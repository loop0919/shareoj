<script setup lang="ts">
const props = defineProps<{ problemId: string, average: number | null, count: number }>()
const { user } = useAccount()
const average = ref(props.average)
const count = ref(props.count)
const selected = ref<number | null>(null)
const voted = ref<number | null>(null)
const loading = ref(false)
const ready = ref(false)
const error = ref('')
const message = ref('')
let requestVersion = 0
async function load() {
  const version = ++requestVersion
  ready.value = false
  error.value = ''; message.value = ''
  selected.value = null; voted.value = null
  average.value = props.average; count.value = props.count
  if (!user.value) { loading.value = false; return }
  loading.value = true
  try {
    const result = await $fetch(`/api/my/difficulty-votes/${props.problemId}`)
    if (version !== requestVersion) return
    selected.value = voted.value = result.difficulty
    average.value = result.difficultyAverage; count.value = result.difficultyVoteCount
    ready.value = true
  } catch { if (version === requestVersion) error.value = '難易度の投票を取得できませんでした。' }
  finally { if (version === requestVersion) loading.value = false }
}
async function save(remove = false) {
  if (!ready.value || loading.value || (!remove && selected.value === null)) return
  const version = requestVersion
  loading.value = true; error.value = ''; message.value = ''
  try {
    const result = await $fetch(`/api/my/difficulty-votes/${props.problemId}`, {
      method: remove ? 'DELETE' : 'PUT', body: remove ? undefined : { difficulty: selected.value },
    })
    if (version !== requestVersion) return
    selected.value = voted.value = result.difficulty
    average.value = result.difficultyAverage; count.value = result.difficultyVoteCount
    message.value = remove ? '投票を取り消しました。' : '投票を保存しました。'
  } catch { if (version === requestVersion) error.value = '投票を保存できませんでした。もう一度お試しください。' }
  finally { if (version === requestVersion) loading.value = false }
}
watch(() => [user.value?.id, props.problemId], load)
onMounted(load)
onBeforeUnmount(() => { requestVersion++ })
</script>
<template>
  <section class="difficulty-vote" aria-label="難易度投票">
    <p>難易度（みんなの投票） <strong>{{ average === null ? '未投票' : `Lv.${average.toFixed(1)}` }}</strong> <span class="muted">（{{ count }}票）</span></p>
    <div v-if="user" class="vote-controls">
      <label for="difficulty-vote-level">あなたの評価</label>
      <select id="difficulty-vote-level" v-model="selected" :disabled="loading || !ready">
        <option :value="null" disabled>選択してください</option>
        <option v-for="level in 10" :key="level" :value="level">Lv.{{ level }}</option>
      </select>
      <button class="editor-button" :disabled="loading || !ready || selected === null || selected === voted" @click="save()">{{ voted === null ? '投票する' : '投票を変更' }}</button>
      <button v-if="voted !== null" class="editor-button" :disabled="loading || !ready" @click="save(true)">投票を取り消す</button>
    </div>
    <NuxtLink v-else :to="{ path: '/login', query: { next: `/problems/${problemId}` } }">ログインして難易度を投票</NuxtLink>
    <p v-if="loading || message" role="status">{{ loading ? '読み込み中…' : message }}</p>
    <p v-if="error" role="alert">{{ error }} <button v-if="!ready" class="editor-button" :disabled="loading" @click="load">再試行</button></p>
  </section>
</template>
<style scoped>
.difficulty-vote { margin-block: 16px; font-size: .875rem; }
.difficulty-vote p { margin-block: 8px; }
.vote-controls { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
select { min-height: 36px; max-width: 100%; padding: 6px 8px; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-paper); color: var(--color-ink); font: inherit; }
select:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 3px; }
</style>
