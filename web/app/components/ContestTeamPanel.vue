<script setup lang="ts">
import type { Contest, MyTeam } from '~~/shared/types/contest'
import { contestTeamError, errorCode } from '~/utils/contest-team'
// The viewer's team on the team page of a team contest: members, the join code only members see, renaming, and before
// the start leaving or removing a member (ADR 0024).
const props = defineProps<{ contest: Contest }>()
const emit = defineEmits<{ changed: [] }>()
const { profile } = useAccount()
const { showToast } = useToast()
const base = computed(() => `/api/my/contests/${props.contest.id}/team`)
const me = computed(() => profile.value?.handle ?? '')
const team = ref<MyTeam | null>(null)
// The contest is polled, so its summary is the latest name and member list; the code comes from the team route.
const name = computed(() => props.contest.team?.name ?? team.value?.name ?? '')
const members = computed(() => props.contest.team?.members ?? team.value?.members ?? [])
const beforeStart = computed(() => props.contest.status === 'scheduled')
const running = computed(() => props.contest.status !== 'ended')
const action = ref<'' | 'load' | 'code' | 'name' | 'remove' | 'leave'>('')
const message = ref('')
const failed = ref(false)
const renaming = ref(false)
const newName = ref('')
const copied = ref(false)
// Swapping the form for the button, or removing a member's row, would drop focus to the page; these take it instead.
const renameButton = ref<HTMLButtonElement>()
const renameInput = ref<HTMLInputElement>()
const memberList = ref<HTMLUListElement>()
let copiedTimer: ReturnType<typeof setTimeout> | undefined
onBeforeUnmount(() => clearTimeout(copiedTimer))
function report(text: string, error = false) { message.value = text; failed.value = error }
function teamError(error: unknown) {
  if (errorCode(error) === 'team_locked') {
    return action.value === 'remove' || action.value === 'leave' ? '開始後はメンバーを外せず、チームを抜けることもできません。' : '終了後はチーム名と参加コードを変更できません。'
  }
  return contestTeamError(error, 'チームを更新できませんでした。時間をおいてもう一度お試しください。')
}
async function load() {
  action.value = 'load'
  try { team.value = await $fetch<MyTeam>(base.value) }
  catch (error) { report(contestTeamError(error, '参加コードを取得できませんでした。ページを再読み込みしてください。'), true) }
  finally { action.value = '' }
}
// After the end nobody joins, so the code is neither read nor shown.
onMounted(() => { if (running.value) void load() })
// Removing a member renews the code, so a changed member list from the polled contest rereads it.
watch(() => `${props.contest.team?.id}:${props.contest.team?.members.join(',')}`, (now, old) => { if (now !== old && !action.value && running.value) void load() })
// Another member may renew the code without changing the members, so the panel rereads it quietly as the contest is
// polled; a dead code on screen would be read out to people at the venue.
async function refresh() {
  if (action.value) return
  try {
    const latest = await $fetch<MyTeam>(base.value)
    if (!action.value) team.value = latest
  } catch { /* A removal shows in the polled contest; the next round tries again. */ }
}
usePolling(refresh, 15000, () => running.value)
async function renewCode() {
  if (action.value || !window.confirm('参加コードを作り直しますか？今のコードではチームに入れなくなります。')) return
  action.value = 'code'; report('')
  try {
    team.value = await $fetch<MyTeam>(`${base.value}/code`, { method: 'PUT' })
    report('参加コードを作り直しました。前のコードでは入れません。')
  } catch (error) { report(teamError(error), true) }
  finally { action.value = '' }
}
async function copyCode() {
  try {
    await navigator.clipboard.writeText(team.value?.code ?? '')
    report('')
    copied.value = true
    clearTimeout(copiedTimer)
    copiedTimer = setTimeout(() => { copied.value = false }, 2000)
  } catch { report('参加コードを選択してコピーしてください。', true) }
}
async function startRename() {
  newName.value = name.value; renaming.value = true; report('')
  await nextTick()
  renameInput.value?.focus()
}
async function closeRename() {
  renaming.value = false
  await nextTick()
  renameButton.value?.focus()
}
async function rename() {
  if (action.value) return
  action.value = 'name'; report('')
  let renamed = false
  try {
    team.value = await $fetch<MyTeam>(`${base.value}/name`, { method: 'PUT', body: { name: newName.value } })
    report('チーム名を変更しました。')
    emit('changed')
    renamed = true
  } catch (error) { report(teamError(error), true) }
  finally { action.value = '' }
  // The button is enabled again only now, so it can take focus.
  if (renamed) await closeRename()
}
async function remove(handle: string) {
  if (action.value || !window.confirm(`${handle}をチームから外しますか？外した人の参加登録は取り消され、参加コードは作り直されます。`)) return
  action.value = 'remove'; report('')
  try {
    await $fetch(`${base.value}/members/${encodeURIComponent(handle)}`, { method: 'DELETE' })
    report(`${handle}をチームから外しました。参加コードを作り直しました。`)
    emit('changed')
    // The focused 「外す」 button goes with the row once the contest is read again.
    memberList.value?.focus()
    action.value = ''
    await load()
  } catch (error) { report(teamError(error), true) }
  finally { action.value = '' }
}
async function leave() {
  if (action.value || !window.confirm(members.value.length > 1 ? 'チームを抜けますか？参加登録も取り消されます。' : 'チームを抜けますか？参加登録も取り消され、メンバーがいなくなるのでチームはなくなります。')) return
  action.value = 'leave'; report('')
  try {
    await $fetch(`${base.value}/members/${encodeURIComponent(me.value)}`, { method: 'DELETE' })
    emit('changed')
    showToast('チームを抜けました')
  } catch (error) { report(teamError(error), true) }
  finally { action.value = '' }
}
</script>

<template>
  <section class="team-panel" aria-labelledby="team-panel-title">
    <h2 id="team-panel-title">チーム</h2>
    <div v-if="!renaming" class="team-name"><strong>{{ name }}</strong><button v-if="running" ref="renameButton" type="button" class="editor-button" :disabled="!!action" @click="startRename">チーム名を変更</button></div>
    <form v-else class="team-rename" @submit.prevent="rename">
      <label for="team-new-name">新しいチーム名</label>
      <div class="team-row"><input id="team-new-name" ref="renameInput" v-model="newName" required autocomplete="off"><button type="submit" class="editor-button" :disabled="!!action" :aria-busy="action === 'name'">{{ action === 'name' ? '保存中…' : '保存' }}</button><button type="button" class="editor-button" :disabled="action === 'name'" @click="closeRename">キャンセル</button></div>
    </form>
    <ul ref="memberList" class="team-members" aria-label="メンバー" tabindex="-1">
      <li v-for="handle in members" :key="handle">
        <span class="team-member"><UserLink :handle="handle" /><span v-if="handle === me" class="team-tag">自分</span></span>
        <button v-if="beforeStart && handle !== me" type="button" class="editor-button" :disabled="!!action" :aria-label="`${handle}をチームから外す`" @click="remove(handle)">外す</button>
      </li>
    </ul>
    <div v-if="running" class="team-code">
      <label for="team-join-code">参加コード</label>
      <div class="team-row">
        <input id="team-join-code" :value="team?.code ?? ''" readonly :placeholder="action === 'load' ? '読み込み中…' : ''" aria-describedby="team-code-help" @focus="($event.target as HTMLInputElement).select()">
        <button type="button" class="editor-button team-copy" :disabled="!team" :aria-label="copied ? 'コピーしました' : '参加コードをコピー'" :title="copied ? 'コピーしました' : '参加コードをコピー'" @click="copyCode"><svg v-if="copied" class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12 5 5L19 8" /></svg><svg v-else class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="8" y="8" width="12" height="12" rx="2" /><path d="M16 8V4H4v12h4" /></svg></button>
        <button type="button" class="editor-button" :disabled="!!action || !team" :aria-busy="action === 'code'" @click="renewCode">{{ action === 'code' ? '作り直し中…' : '作り直す' }}</button>
      </div>
      <p id="team-code-help" class="muted">参加コードを伝えた人は、「チーム参加」の「チームに入る」から加われます。開始後はメンバーを外せませんが、参加コードを作り直すと、それ以降の加入を止められます。</p>
    </div>
    <p v-else class="muted">終了したため、チームには入れません。</p>
    <button v-if="beforeStart" type="button" class="editor-button danger team-leave" :disabled="!!action" @click="leave">チームを抜ける</button>
    <p v-if="message" :class="failed ? 'notice notice-error' : 'team-message'" :role="failed ? 'alert' : 'status'">{{ message }}</p>
  </section>
</template>

<style scoped>
/* Hallmark · component: team panel · genre: modern-minimal · theme: existing ShareOJ tokens (Plain)
 * states: loading · default · renaming · busy · copied · error · locked after the start or the end
 * A quiet bordered block: name, members as a ruled list, then the code row, as in the co-setter panel. */
.team-panel { display: grid; gap: 12px; max-width: 36rem; margin-block: 16px 0; padding: 16px 20px; border: 1px solid var(--color-line); border-radius: 4px; }
.team-panel h2 { margin: 0; font-size: 1rem; }
.team-panel p { margin: 0; font-size: .8125rem; }
.team-panel .editor-button { min-height: 44px; padding: 8px 14px; color: var(--color-ink); border-color: var(--color-line); }
.team-panel .editor-button.danger { color: var(--color-error); border-color: var(--color-error); }
.team-name { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px; }
.team-name strong { min-width: 0; font-size: 1.125rem; overflow-wrap: anywhere; }
.team-rename, .team-code { display: grid; gap: 6px; }
.team-rename label, .team-code label { font-size: .8125rem; font-weight: 600; }
.team-row { display: flex; flex-wrap: wrap; gap: 8px; }
.team-row input { flex: 1 1 12rem; min-width: 0; min-height: 44px; padding: 8px 10px; border: 1px solid var(--color-line); border-radius: 4px; font: inherit; }
.team-row input:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 2px; }
#team-join-code { font-family: var(--font-code); letter-spacing: .06em; }
.team-copy { display: inline-flex; align-items: center; justify-content: center; width: 44px; padding: 0 !important; }
.team-members { display: grid; margin: 0; padding: 0; list-style: none; border-top: 1px solid var(--color-line); }
.team-members li { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px; min-height: 44px; padding: 4px 0; border-bottom: 1px solid var(--color-line); font-size: .875rem; }
.team-member { display: inline-flex; align-items: center; gap: 8px; min-width: 0; overflow-wrap: anywhere; }
.team-tag { padding: 1px 6px; border: 1px solid var(--color-line); border-radius: 4px; color: var(--color-muted); font-size: .75rem; white-space: nowrap; }
.team-leave { justify-self: start; }
.team-message { color: var(--color-muted); }
.team-panel button:disabled { opacity: .6; cursor: not-allowed; }
</style>
