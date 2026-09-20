<script setup lang="ts">
const props = defineProps<{ problemId: string, average: number | null, count: number }>()
const emit = defineEmits<{ updated: [average: number | null, count: number] }>()
const dialog = ref<HTMLDialogElement>()
function open() {
  selected.value = voted.value
  message.value = ''
  dialog.value?.showModal()
}
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
    emit('updated', average.value, count.value)
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
    emit('updated', average.value, count.value)
    dialog.value?.close()
    message.value = remove ? '投票を取り消しました。' : '投票を保存しました。'
  } catch { if (version === requestVersion) error.value = '投票を保存できませんでした。もう一度お試しください。' }
  finally { if (version === requestVersion) loading.value = false }
}
watch(() => [user.value?.id, props.problemId], ([owner, id], [previousOwner, previousId]) => {
  if ((previousOwner && owner !== previousOwner) || id !== previousId) dialog.value?.close()
  void load()
})
onMounted(load)
onBeforeUnmount(() => { requestVersion++ })
</script>
<template>
  <div class="difficulty-vote">
    <button type="button" class="editor-button" aria-haspopup="dialog" @click="open">難易度評価</button>
    <span class="sr-only" role="status">{{ message }}</span>
    <Teleport to="body">
      <dialog ref="dialog" class="difficulty-dialog" aria-labelledby="difficulty-vote-title" @cancel="loading && $event.preventDefault()">
        <header>
          <h2 id="difficulty-vote-title">難易度評価</h2>
          <button type="button" class="editor-button close-button" aria-label="閉じる" :disabled="loading" @click="dialog?.close()">×</button>
        </header>
        <p class="muted">難易度を1〜10段階で評価できます。投票はあとから変更・取り消しできます。</p>
        <p>みんなの投票 <strong>{{ average === null ? '未投票' : `Lv.${average.toFixed(1)}` }}</strong> <span class="muted">（{{ count }}票）</span></p>
        <template v-if="user">
          <label for="difficulty-vote-level">あなたの評価</label>
          <select id="difficulty-vote-level" v-model="selected" :disabled="loading || !ready">
            <option :value="null" disabled>選択してください</option>
            <option v-for="level in 10" :key="level" :value="level">Lv.{{ level }}</option>
          </select>
          <p v-if="loading" role="status">読み込み中…</p>
          <p v-if="error" role="alert">{{ error }} <button v-if="!ready" class="editor-button" :disabled="loading" @click="load">再試行</button></p>
          <footer>
            <button v-if="voted !== null" class="editor-button withdraw" :disabled="loading || !ready" @click="save(true)">投票を取り消す</button>
            <button class="editor-button primary" :disabled="loading || !ready || selected === null || selected === voted" @click="save()">{{ voted === null ? '投票する' : '投票を変更' }}</button>
          </footer>
        </template>
        <NuxtLink v-else :to="{ path: '/login', query: { next: `/problems/${problemId}` } }">ログインして難易度を投票</NuxtLink>
      </dialog>
    </Teleport>
  </div>
</template>
<style scoped>
/* Hallmark · component: difficulty dialog · theme: existing project tokens
 * pre-emit critique: P4 H4 E4 S4 R5 V4 */
.sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; border: 0; }
.difficulty-vote { font-size: .875rem; }
.editor-button { white-space: nowrap; }
.difficulty-dialog { width: min(460px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow: auto; padding: 24px; border: 1px solid var(--color-line); border-radius: 8px; color: var(--color-ink); background: var(--color-paper); text-align: left; }
.difficulty-dialog::backdrop { background: var(--color-dialog-backdrop); }
header { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-bottom: 16px; }
h2 { margin: 0; font-size: 1.25rem; }
p { margin: 0 0 20px; font-size: .875rem; line-height: 1.8; }
.close-button { width: 36px; height: 36px; padding: 0; font-size: 1.5rem; }
label { display: block; margin-bottom: 8px; font-size: .875rem; font-weight: 600; }
select { width: 100%; min-height: 44px; padding: 10px 12px; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-paper); color: var(--color-ink); font: inherit; }
select:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 2px; }
footer { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 12px; margin-top: 24px; padding-top: 20px; border-top: 1px solid var(--color-line); }
.withdraw { margin-right: auto; }
</style>
