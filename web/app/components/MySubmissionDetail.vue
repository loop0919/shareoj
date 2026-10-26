<script setup lang="ts">
import { contestSubmissionsPath } from '~~/shared/types/contest'
import type { Submission } from '~~/shared/types/submission'
import { shortId } from '~~/shared/utils/short-id'
const props = defineProps<{ id: string, contestId?: string }>()
useSeoMeta({ title: '提出結果 | ShareOJ', robots: 'noindex, nofollow' })
const route = useRoute()
const { user } = useAccount()
const item = ref<Submission | null>(null)
const message = ref('')
const { loading, run } = useLatestRequest()
function load() {
  message.value = ''
  const url = `/api/my/submissions/${encodeURIComponent(props.id)}`
  return run(url, async () => {
    try { return { submission: await $fetch<Submission>(url), shared: false } }
    catch (error) {
      if (![401, 403, 404].includes((error as { statusCode?: number }).statusCode ?? 0)) throw error
      return { submission: await $fetch<Submission>(`/api/submissions/${encodeURIComponent(props.id)}`), shared: true }
    }
  }, ({ submission, shared }) => {
    if (shared) {
      const path = submission.contestId ? `/contests/${shortId(submission.contestId)}` : `/problems/${shortId(submission.problemId)}`
      void navigateTo(`${path}/submissions/${shortId(submission.id)}`, { replace: true })
    } else item.value = submission
  }, error => {
    message.value = (error as { statusCode?: number }).statusCode === 404
      ? '提出が見つからないか、まだ公開されていません。自分の提出はログインして確認してください。'
      : '提出結果を取得できませんでした。ログイン状態を確認して、再取得してください。'
  })
}
// The problem's own list: in a contest, the contest's narrowed to the problem (ADR 0034).
const problemList = computed(() => {
  const submission = item.value
  if (!submission) return ''
  const contest = props.contestId || (route.query.from === 'contest-problem' ? submission.contestId : '')
  if (!contest) return `/problems/${shortId(submission.problemId)}?view=my-submissions`
  return contestSubmissionsPath(contest, 'my-submissions', submission.contestId === contest ? submission.problemIndex : undefined)
})
onMounted(load)
usePolling(load, 2000, () => !message.value && (!!item.value && item.value.status !== 'DONE'))
</script>

<template>
  <div>
    <nav class="breadcrumb" aria-label="パンくずリスト"><NuxtLink v-if="contestId && (route.query.from === 'contest' || route.query.from === 'contest-team')" :to="`/contests/${shortId(contestId)}?view=my-submissions`">{{ route.query.from === 'contest-team' ? 'コンテストのチームの提出' : 'コンテストの自分の提出' }}</NuxtLink><NuxtLink v-else-if="item && (contestId || route.query.from === 'problem' || route.query.from === 'contest-problem')" :to="problemList">{{ contestId && route.query.from === 'team' ? 'この問題のチームの提出' : 'この問題の自分の提出' }}</NuxtLink><NuxtLink v-else to="/my/submissions">提出履歴</NuxtLink><span aria-hidden="true">/</span><span>提出結果</span></nav>
    <p v-if="message" class="notice notice-error" role="alert">{{ message }} <NuxtLink v-if="!user" :to="{ path: '/login', query: { next: route.fullPath } }">ログイン</NuxtLink> <button class="editor-button" @click="load">再取得</button></p>
    <SubmissionDetail v-if="item" :item="item" @rejudged="load" />
    <LoadingSpinner v-else-if="!message" label="提出結果を読み込み中" />
  </div>
</template>
