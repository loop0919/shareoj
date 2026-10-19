<script setup lang="ts">
import type { Contest, JoinBody } from '~~/shared/types/contest'
import { shortId } from '~~/shared/utils/short-id'
import { contestTeamError } from '~/utils/contest-team'
// detailed is the overview page: only it takes the venue passphrase and holds the team forms and panel (ADR 0024).
const props = defineProps<{ contest: Contest, detailed?: boolean }>()
const emit = defineEmits<{ joined: [contest: Contest], changed: [] }>()
const { user } = useAccount()
const { showToast } = useToast()
const route = useRoute()
const saving = ref(false)
const error = ref('')
const teamMode = computed(() => props.contest.teamMode === true)
const division = computed(() => props.contest.onsiteDivision === true)
// A registration without a team (left by the old API during a deploy) still has to create or join one.
const member = computed(() => props.contest.participating && (!teamMode.value || !!props.contest.team))
const teamAction = ref<'create' | 'join'>('create')
const teamActions = [{ value: 'create' as const, label: 'チームを作る' }, { value: 'join' as const, label: 'チームに入る' }]
const teamName = ref('')
const teamCode = ref('')
const passphrase = ref('')
const venue = ref('')
const fallback = '参加登録できませんでした。ログイン状態・開催期間を確認し、再読み込みしてお試しください。'
// A wrong code or passphrase registers nothing, so the inputs stay for another try.
async function send(body?: JoinBody) {
  saving.value = true
  error.value = ''
  try {
    return await $fetch<Contest>(`/api/my/contests/${props.contest.id}/participation`, { method: 'POST', ...body ? { body } : {} })
  } catch (failure) {
    error.value = contestTeamError(failure, fallback)
    return undefined
  } finally { saving.value = false }
}
async function join() {
  if (saving.value) return
  const body: JoinBody = {}
  if (teamMode.value) body.team = teamAction.value === 'create' ? { action: 'create', name: teamName.value } : { action: 'join', code: teamCode.value }
  if (props.detailed && division.value && passphrase.value.trim()) body.onsitePassphrase = passphrase.value
  // Without a team or a passphrase the request stays empty, as from the pages before them.
  const result = await send(Object.keys(body).length ? body : undefined)
  if (!result) return
  teamName.value = ''; teamCode.value = ''; passphrase.value = ''
  emit('joined', result)
}
async function goOnsite() {
  if (saving.value) return
  // required accepts spaces, which the server trims away; such a request could only change nothing.
  if (!venue.value.trim()) { error.value = '会場の合言葉を入力してください。'; return }
  const result = await send({ onsitePassphrase: venue.value })
  if (!result) return
  emit('joined', result)
  // Success is what the contest says now, so the page never reports onsite while the participant stays open.
  if (!result.onsite) { error.value = 'オンサイト参加に変わりませんでした。ページを再読み込みしてから、もう一度お試しください。'; return }
  venue.value = ''
  showToast('オンサイト参加に変わりました')
}
</script>

<template>
  <div class="contest-participation">
    <!-- A public response (for example, before the profile exists) says nothing about the viewer, so it claims no role. -->
    <p v-if="user && contest.excluded" class="muted">このコンテストのスタッフを務めたため、参加できません。</p>
    <p v-else-if="user && contest.staff" class="muted">スタッフ（作成者・作問者・テスター）は公式参加の対象外です。</p>
    <template v-else-if="user && member">
      <p class="participation-state"><span class="participating" role="status">参加済み</span><span v-if="division" class="participation-division" :data-onsite="contest.onsite || undefined">{{ contest.onsite ? 'オンサイト参加' : 'オープン参加' }}</span><span v-if="contest.team && !detailed" class="participation-team">チーム <strong>{{ contest.team.name }}</strong></span></p>
      <ContestTeamPanel v-if="detailed && contest.team" :contest="contest" @changed="emit('changed')" />
      <form v-if="detailed && division && !contest.onsite && contest.status !== 'ended'" class="participation-form" @submit.prevent="goOnsite">
        <div class="participation-field"><label for="venue-passphrase">会場の合言葉（オンサイト参加者のみ）</label><input id="venue-passphrase" v-model="venue" required autocomplete="off" autocapitalize="off" spellcheck="false" aria-describedby="venue-note"></div>
        <button type="submit" class="editor-button" :disabled="saving" :aria-busy="saving">{{ saving ? '確認中…' : 'オンサイト参加にする' }}</button>
        <p id="venue-note" class="muted">会場で配られた合言葉を入れると、終了までオンサイト参加に変えられます。<template v-if="teamMode">メンバーの1人が入れると、チーム全体がオンサイト参加になります。</template>オンサイト参加は順位表に表示されます。</p>
      </form>
    </template>
    <p v-else-if="contest.status === 'ended'" class="muted">参加受付は終了しました。</p>
    <template v-else-if="!user">
      <NuxtLink class="editor-button" :to="{ path: '/login', query: { next: route.fullPath } }">ログインして参加する</NuxtLink>
      <p class="muted">参加すると順位表に表示されます。</p>
    </template>
    <template v-else-if="teamMode && !detailed">
      <NuxtLink class="editor-button" :to="`/contests/${shortId(contest.id)}`">チームを作る・チームに入る</NuxtLink>
      <p class="muted">チーム戦です。概要ページでチームを作るか、参加コードでチームに入って参加します。</p>
    </template>
    <form v-else class="participation-form" @submit.prevent="join">
      <template v-if="teamMode">
        <div class="participation-choice"><SegmentedChoice v-model="teamAction" legend="参加のしかた" name="team-action" :options="teamActions" hide-legend /></div>
        <div v-if="teamAction === 'create'" class="participation-field"><label for="team-name">チーム名</label><input id="team-name" v-model="teamName" required autocomplete="off"></div>
        <div v-else class="participation-field"><label for="team-code">チームの参加コード</label><input id="team-code" v-model="teamCode" required maxlength="64" autocomplete="off" autocapitalize="characters" spellcheck="false" aria-describedby="team-code-note"><p id="team-code-note" class="muted">チームのメンバーに参加コードを聞いて入力します。</p></div>
      </template>
      <div v-if="detailed && division" class="participation-field"><label for="join-passphrase">会場の合言葉（オンサイト参加者のみ）</label><input id="join-passphrase" v-model="passphrase" autocomplete="off" autocapitalize="off" spellcheck="false" aria-describedby="join-passphrase-note"><p id="join-passphrase-note" class="muted">入れるとオンサイト参加、空のままならオープン参加です。オンサイト参加は順位表に表示されます。</p></div>
      <button type="submit" class="editor-button" :disabled="saving" :aria-busy="saving">{{ saving ? '参加登録中…' : '参加する' }}</button>
      <p class="muted">参加すると順位表に表示されます。<template v-if="teamMode">1人で出るときも、1人のチームを作ります。<template v-if="contest.maxTeamSize">1チーム{{ contest.maxTeamSize }}人までです。</template>開始後はチームを抜けられず、メンバーも外せません。</template><template v-if="division && !detailed">オンサイト参加の人は、概要ページで会場の合言葉を入れて参加してください。</template></p>
    </form>
    <p v-if="error" class="notice notice-error" role="alert">{{ error }}</p>
  </div>
</template>

<style scoped>
/* Hallmark · component: contest participation · genre: modern-minimal · theme: existing ShareOJ tokens (Plain)
 * states: guest · staff · open · onsite · team forms · busy · error
 * One column of labelled fields; the choice, inputs and button share one width so the form reads top to bottom. */
.contest-participation { margin-block: 24px; }
.contest-participation p { margin: 8px 0; }
.contest-participation .editor-button { min-height: 44px; padding: 8px 20px; color: var(--color-accent); border-color: var(--color-accent); }
.contest-participation button:disabled { opacity: .6; cursor: wait; }
.participation-state { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 12px; }
.participating { color: var(--color-accent); font-weight: 600; }
.participation-division { padding: 2px 8px; border: 1px solid var(--color-line); border-radius: 4px; color: var(--color-muted); font-size: .8125rem; }
.participation-division[data-onsite] { border-color: var(--color-accent); color: var(--color-accent); background: var(--color-accent-soft); }
.participation-team { color: var(--color-muted); font-size: .875rem; overflow-wrap: anywhere; }
.participation-form { display: grid; gap: 12px; max-width: 28rem; margin-block: 16px 0; }
.participation-form > .editor-button { justify-self: start; }
.participation-field { display: grid; gap: 6px; min-width: 0; }
.participation-field label { font-size: .875rem; font-weight: 600; }
.participation-field p { margin: 0; font-size: .8125rem; }
.participation-field input { width: 100%; min-width: 0; min-height: 44px; padding: 8px 10px; border: 1px solid var(--color-line); border-radius: 4px; font: inherit; }
.participation-field input:hover { border-color: var(--color-muted); }
.participation-field input:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 2px; }
#team-code { font-family: var(--font-code); letter-spacing: .04em; }
</style>
