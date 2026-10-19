<script setup lang="ts">
import type { Contest, JoinBody } from '~~/shared/types/contest'
import { shortId } from '~~/shared/utils/short-id'
import { venuePassphraseRule } from '~/utils/venue-passphrase'
// The team page's form: create a team or join one by its code, with the venue passphrase when the contest has a
// division. Someone registered alone uses it to switch to a team before the start (ADR 0024).
const props = defineProps<{ contest: Contest }>()
const emit = defineEmits<{ joined: [contest: Contest] }>()
const { saving, error, send } = useContestJoin(() => props.contest.id)
const individual = computed(() => props.contest.participating && !props.contest.team)
// An onsite individual's team turns onsite with them, so there is nothing to enter.
const askPassphrase = computed(() => props.contest.onsiteDivision === true && !(individual.value && props.contest.onsite))
const teamAction = ref<'create' | 'join'>('create')
const teamActions = [{ value: 'create' as const, label: 'チームを作る' }, { value: 'join' as const, label: 'チームに入る' }]
const teamName = ref('')
const teamCode = ref('')
const carried = useCarriedPassphrase(props.contest.id)
const passphrase = ref(carried.value)
carried.value = ''
// A wrong code or passphrase registers nothing, so the inputs stay for another try.
async function join() {
  if (saving.value) return
  const body: JoinBody = { team: teamAction.value === 'create' ? { action: 'create', name: teamName.value } : { action: 'join', code: teamCode.value } }
  if (askPassphrase.value && passphrase.value) body.onsitePassphrase = passphrase.value
  const result = await send(body)
  if (!result) return
  teamName.value = ''; teamCode.value = ''; passphrase.value = ''
  emit('joined', result)
}
</script>

<template>
  <form class="participation-form" @submit.prevent="join">
    <p v-if="individual" class="notice">個人参加で登録しています。チームを作るかチームに入ると、個人参加の登録はチームのメンバーとしての登録に置き換わります。<template v-if="contest.onsite">オンサイト参加なので、チームもオンサイト参加になります。</template>開始後は切り替えられません。</p>
    <div class="participation-choice"><SegmentedChoice v-model="teamAction" legend="参加のしかた" name="team-action" :options="teamActions" hide-legend /></div>
    <div v-if="teamAction === 'create'" class="participation-field"><label for="team-name">チーム名</label><input id="team-name" v-model="teamName" required autocomplete="off"></div>
    <div v-else class="participation-field"><label for="team-code">チームの参加コード</label><input id="team-code" v-model="teamCode" required maxlength="64" autocomplete="off" autocapitalize="characters" spellcheck="false" aria-describedby="team-code-note"><p id="team-code-note" class="muted">チームのメンバーに参加コードを聞いて入力します。</p></div>
    <div v-if="askPassphrase" class="participation-field"><label for="join-passphrase">会場の合言葉（オンサイト参加者のみ）</label><VenuePassphraseInput id="join-passphrase" v-model="passphrase" aria-describedby="join-passphrase-note" /><p id="join-passphrase-note" class="muted">{{ venuePassphraseRule }}です。入れるとチームがオンサイト参加になります。オンサイト参加は順位表に表示されます。</p></div>
    <button type="submit" class="editor-button" :disabled="saving" :aria-busy="saving">{{ saving ? individual ? '切り替え中…' : '参加登録中…' : individual ? 'チーム参加に切り替える' : '参加する' }}</button>
    <p class="muted"><template v-if="!individual">参加すると順位表に表示されます。</template><template v-if="contest.maxTeamSize">1チーム{{ contest.maxTeamSize }}人までです。</template>開始後はチームを抜けられず、メンバーも外せません。<template v-if="!individual">1人で出るときは、<NuxtLink :to="`/contests/${shortId(contest.id)}`">概要ページ</NuxtLink>の「個人参加」から参加できます。</template></p>
    <p v-if="error" class="notice notice-error" role="alert">{{ error }}</p>
  </form>
</template>
