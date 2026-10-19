<script setup lang="ts">
import { fullId, shortId } from '~~/shared/utils/short-id'
// The team page of a team contest (ADR 0024): create a team or join one, switch to a team from an individual
// registration before the start, and as a member manage the team.
definePageMeta({ key: route => route.path })
useResponseHeader('Cache-Control').value = 'no-store'
useResponseHeader('Vary').value = 'Cookie'
const route = useRoute()
const { user } = useAccount()
const id = fullId(route.params.id)
const { data: contest, error, refresh } = await useContest(id)
if (error.value || !contest.value) throw createError({ statusCode: error.value?.statusCode === 404 ? 404 : 502, statusMessage: 'コンテストを取得できませんでした', fatal: true })
useSeoMeta({ title: () => `チーム参加 | ${contest.value?.title} | ShareOJ`, robots: 'noindex' })
const overview = `/contests/${shortId(id)}`
const member = computed(() => !!contest.value?.participating && !!contest.value.team)
const individual = computed(() => !!contest.value?.participating && !contest.value.team)
// The viewer's role decides the page, and members follow joins, renames and removals by the others.
watch(() => user.value?.id, () => { void refresh() })
usePolling(() => refresh(), 15000)
</script>

<template>
  <div v-if="contest" class="problem-page contest-page">
    <div class="breadcrumb-row"><nav class="breadcrumb" aria-label="パンくずリスト"><NuxtLink :to="overview">{{ contest.title }}</NuxtLink><span aria-hidden="true">/</span><span>チーム参加</span></nav></div>
    <section class="contest-section contest-team-page" aria-labelledby="team-page-title">
      <h1 id="team-page-title">チーム参加</h1>
      <p v-if="!contest.teamMode" class="notice">このコンテストは個人戦です。<NuxtLink :to="overview">概要ページ</NuxtLink>から参加してください。</p>
      <div v-else class="contest-participation">
        <p v-if="user && contest.excluded" class="muted">このコンテストのスタッフを務めたため、参加できません。</p>
        <p v-else-if="user && contest.staff" class="muted">スタッフ（作成者・作問者・テスター）は公式参加の対象外です。</p>
        <template v-else-if="user && member">
          <ContestParticipationState :contest="contest" hide-team />
          <ContestTeamPanel :contest="contest" @changed="refresh()" />
          <ContestOnsiteUpgrade v-if="contest.onsiteDivision && !contest.onsite && contest.status !== 'ended'" :contest="contest" @joined="value => contest = value" />
        </template>
        <template v-else-if="user && individual && contest.status !== 'scheduled'">
          <ContestParticipationState :contest="contest" />
          <p class="muted">開始後は、個人参加からチーム参加に切り替えられません。</p>
        </template>
        <p v-else-if="contest.status === 'ended'" class="muted">参加受付は終了しました。</p>
        <template v-else-if="!user">
          <NuxtLink class="editor-button" :to="{ path: '/login', query: { next: route.fullPath } }">ログインしてチーム参加する</NuxtLink>
          <p class="muted">ログインしてから、チームを作るか、参加コードでチームに入ります。</p>
        </template>
        <ContestTeamJoin v-else :contest="contest" @joined="value => contest = value" />
      </div>
    </section>
  </div>
</template>

<style scoped>
/* Hallmark · component: team page · genre: modern-minimal · theme: existing ShareOJ tokens (Plain)
 * states: guest · staff · not a team contest · form · individual before and after the start · member · ended
 * The contest page's section rhythm: breadcrumb, a section title as the page heading, then the participation column. */
.contest-page { padding-bottom: 64px; }
.contest-section { padding-block: 32px; }
.contest-section h1 { margin: 0 0 20px; font-size: 1.25rem; letter-spacing: normal; }
.contest-team-page .contest-participation { max-width: 36rem; margin-block: 0; }
</style>
