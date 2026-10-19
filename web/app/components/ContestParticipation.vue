<script setup lang="ts">
import type { Contest, JoinBody } from '~~/shared/types/contest'
import { shortId } from '~~/shared/utils/short-id'
import { venuePassphraseBlank, venuePassphraseRule } from '~/utils/venue-passphrase'
// detailed is the overview page: only it takes the venue passphrase and offers the switch to a team. Team contests join
// alone here with 「個人参加」; 「チーム参加」 leads to the team page, which forms teams and holds the team panel (ADR 0024).
const props = defineProps<{ contest: Contest, detailed?: boolean }>()
const emit = defineEmits<{ joined: [contest: Contest] }>()
const { user } = useAccount()
const route = useRoute()
const teamMode = computed(() => props.contest.teamMode === true)
const division = computed(() => props.contest.onsiteDivision === true)
const teamPage = computed(() => `/contests/${shortId(props.contest.id)}/team`)
const login = (next: string) => ({ path: '/login', query: { next } })
const { saving, error, send } = useContestJoin(() => props.contest.id)
const passphrase = ref('')
const carried = useCarriedPassphrase(props.contest.id)
// A wrong passphrase registers nothing, so the input stays for another try.
async function join() {
  if (saving.value) return
  const body: JoinBody = {}
  if (props.detailed && division.value && !venuePassphraseBlank(passphrase.value)) body.onsitePassphrase = passphrase.value
  const result = await send(body)
  if (!result) return
  passphrase.value = ''
  emit('joined', result)
}
</script>

<template>
  <div class="contest-participation">
    <!-- A public response (for example, before the profile exists) says nothing about the viewer, so it claims no role. -->
    <p v-if="user && contest.excluded" class="muted">このコンテストのスタッフを務めたため、参加できません。</p>
    <p v-else-if="user && contest.staff" class="muted">スタッフ（作成者・作問者・テスター）は公式参加の対象外です。</p>
    <template v-else-if="user && contest.participating">
      <ContestParticipationState :contest="contest" />
      <template v-if="detailed && teamMode">
        <template v-if="contest.team"><NuxtLink class="editor-button" :to="teamPage">チームのページ</NuxtLink><p class="muted">メンバーと参加コードは、チームのページで確認できます。</p></template>
        <!-- Once it starts, an individual's submissions are their own and cannot move to a team. -->
        <template v-else-if="contest.status === 'scheduled'"><NuxtLink class="editor-button" :to="teamPage">チーム参加に切り替える</NuxtLink><p class="muted">チームを作るかチームに入ると、個人参加の登録はチームのメンバーとしての登録に置き換わります。開始後は切り替えられません。</p></template>
      </template>
      <ContestOnsiteUpgrade v-if="detailed && division && !contest.onsite && contest.status !== 'ended'" :contest="contest" @joined="value => emit('joined', value)" />
    </template>
    <p v-else-if="contest.status === 'ended'" class="muted">参加受付は終了しました。</p>
    <template v-else-if="!user">
      <div v-if="teamMode" class="participation-buttons"><NuxtLink class="editor-button" :to="login(route.fullPath)">個人参加</NuxtLink><NuxtLink class="editor-button" :to="login(teamPage)">チーム参加</NuxtLink></div>
      <NuxtLink v-else class="editor-button" :to="login(route.fullPath)">ログインして参加する</NuxtLink>
      <p class="muted"><template v-if="teamMode">ログインしてから参加します。</template>参加すると順位表に表示されます。</p>
    </template>
    <form v-else class="participation-form" @submit.prevent="join">
      <div v-if="detailed && division" class="participation-field"><label for="join-passphrase">会場の合言葉（オンサイト参加者のみ）</label><VenuePassphraseInput id="join-passphrase" v-model="passphrase" aria-describedby="join-passphrase-note" /><p id="join-passphrase-note" class="muted">{{ venuePassphraseRule }}です。入れるとオンサイト参加、空のままならオープン参加です。オンサイト参加は順位表に表示されます。</p></div>
      <div v-if="teamMode" class="participation-buttons">
        <button type="submit" class="editor-button" :disabled="saving" :aria-busy="saving">{{ saving ? '参加登録中…' : '個人参加' }}</button>
        <NuxtLink class="editor-button" :to="teamPage" @click="carried = passphrase">チーム参加</NuxtLink>
      </div>
      <button v-else type="submit" class="editor-button" :disabled="saving" :aria-busy="saving">{{ saving ? '参加登録中…' : '参加する' }}</button>
      <p class="muted">参加すると順位表に表示されます。<template v-if="teamMode">「チーム参加」では、チームのページでチームを作るか、参加コードでチームに入ります。<template v-if="contest.maxTeamSize">1チーム{{ contest.maxTeamSize }}人までです。</template></template><template v-if="division && !detailed">オンサイト参加の人は、{{ teamMode ? '概要ページかチームのページ' : '概要ページ' }}で会場の合言葉を入れて参加してください。</template></p>
    </form>
    <p v-if="error" class="notice notice-error" role="alert">{{ error }}</p>
  </div>
</template>
