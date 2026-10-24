<script setup lang="ts">
import { clampPage, lastPage, pageOffset } from '~/utils/pagination'
import type { PageSize } from '~/composables/usePageSize'
import { shortId } from '~~/shared/utils/short-id'
// Each tag gets a fresh list and pagination.
definePageMeta({ key: route => `problems:${typeof route.query.tag === 'string' ? route.query.tag : ''}` })
const { user } = useAccount()
const solved = ref(new Set<string>())
const solvedMessage = ref('')
let solvedRequest = 0
async function loadSolved() {
  const version = ++solvedRequest
  solved.value = new Set()
  solvedMessage.value = ''
  if (!user.value) return
  try {
    const result = await $fetch('/api/my/solved-problems')
    if (version === solvedRequest) solved.value = new Set(result.items)
  } catch {
    if (version === solvedRequest) solvedMessage.value = '正解状況を取得できませんでした。'
  }
}
watch(() => user.value?.id, loadSolved)
onMounted(loadSolved)
onBeforeUnmount(() => { solvedRequest++ })
const route = useRoute()
// Older links opened the posting modal here; posting now has its own page.
if (route.query.post === '1') await navigateTo({ path: '/problems/post', query: typeof route.query.problem === 'string' ? { problem: route.query.problem } : {} }, { replace: true, redirectCode: 301 })
const config = useRuntimeConfig()
// Rendering depends on the tag preference and page size cookies.
useResponseHeader('Vary').value = 'Cookie'
const showTags = useProblemTags()
const tag = typeof route.query.tag === 'string' ? route.query.tag : ''
// The page is in the URL and the page size in a cookie, so the server renders the rows the viewer sees (ADR 0028).
const { requested, link, resize } = usePageQuery()
const size = usePageSize('problems')
const offset = computed(() => pageOffset(requested.value, size.value))
// A fixed key, so useFetch reloads once a move has set both the offset and the size: a key made of them is watched
// synchronously and would load the old page at the new size in between. The route catching up with a page already
// requested leaves the offset, and so the list, as it is.
const { data, error, status, refresh } = await useFetch('/api/problems', { key: `problem-list:${tag}`, query: computed(() => ({ ...tag ? { tag } : {}, offset: offset.value, limit: size.value })) })
if (error.value?.statusCode === 400) throw createError({ statusCode: 404, statusMessage: 'Tag not found', fatal: true })
if (error.value || !data.value) throw createError({ statusCode: 502, statusMessage: 'Problem service unavailable', fatal: true })
const last = computed(() => lastPage(data.value?.total ?? 0, size.value))
const page = computed(() => clampPage(requested.value, last.value))
// A page past the end is replaced by the last page, whether the server renders it or the browser moves to it.
const toLastPage = () => data.value && requested.value > last.value ? navigateTo(link(last.value), { replace: true }) : undefined
await toLastPage()
watch(data, () => { void toLastPage() })
const resizeProblems = (to: PageSize) => resize(size, page.value, to)
const date = (value: string) => new Date(value).toLocaleDateString('ja-JP', { timeZone: 'Asia/Tokyo' })
// The canonical is the page on screen; the first page leaves out page.
const path = computed(() => {
  const query = new URLSearchParams({ ...tag ? { tag } : {}, ...page.value > 1 ? { page: String(page.value) } : {} })
  return query.size ? `/problems?${query}` : '/problems'
})
const canonical = computed(() => new URL(path.value, config.public.siteUrl).href)
const heading = tag ? `タグ「${tag}」の問題` : '問題'
const description = tag ? `ShareOJ で「${tag}」のタグが付いたプログラミング問題の一覧。` : 'ShareOJ のプログラミング問題一覧。ユーザーが作成・公開した問題に挑戦し、コードを提出して自動採点できます。'
useSeoMeta({ title: `${heading} | ShareOJ`, description, ogDescription: description, ogTitle: `${heading} | ShareOJ`, ogUrl: canonical, ogType: 'website' })
useHead({ link: [{ rel: 'canonical', href: canonical }] })
useSharePreview({ type: 'website', title: heading, path, description })
</script>
<template>
  <div class="catalogue">
    <header class="catalogue-heading"><h1>問題</h1><NuxtLink class="editor-button primary catalogue-action" to="/problems/post">問題投稿</NuxtLink></header>
    <!-- Viewers who hide problem tags get no tag search, unless a link opened a filtered list. The switch comes first so it stays put when the search appears. -->
    <div class="catalogue-tools">
      <InlineSwitch v-model="showTags" class="tags-switch" :describedby="showTags ? undefined : 'problem-tags-hint'">タグを表示</InlineSwitch>
      <span v-if="!showTags" id="problem-tags-hint" class="tags-hint">解法のヒントになることがあります。</span>
      <TagFilter v-if="showTags || tag" kind="problem" :tag="tag" to="/problems" />
    </div>
    <!-- A page that fails after the first one hides the rows it would replace. -->
    <p v-if="error" role="alert">一覧を取得できませんでした。もう一度お試しください。 <button type="button" class="editor-button" @click="refresh()">再試行</button></p>
    <EmptyState v-else-if="!data?.total" kind="problem" :title="tag ? `「${tag}」のタグが付いた問題はまだありません。` : '公開された問題はまだありません。'" />
    <template v-else>
    <ListToolbar :total="data.total" :page="page" :size="size" unit="問" :link="link" @update:size="resizeProblems" />
    <div class="content-table-scroll" tabindex="0" role="region" aria-label="問題一覧" :aria-busy="status === 'pending'">
      <table class="content-table">
        <thead><tr><th scope="col">タイトル</th><th scope="col">作成者</th><th scope="col">公開日</th><th scope="col"><abbr title="実行時間制限 / メモリ制限">TL / ML</abbr></th><th scope="col">正解者数</th><th scope="col"><abbr title="お気に入り数">Fav</abbr></th><th scope="col">難易度</th></tr></thead>
        <tbody><tr v-for="problem in data.items" :key="problem.id" :class="{ solved: solved.has(problem.id) }">
          <th scope="row"><NuxtLink :to="`/problems/${shortId(problem.id)}`" :aria-label="solved.has(problem.id) ? `${problem.title}（AC 済み）` : undefined">{{ problem.title }}</NuxtLink><TagLinks v-if="showTags && problem.tags.length" class="problem-row-tags" :tags="problem.tags" to="/problems" /></th>
          <td><UserLink :handle="problem.author" /></td><td><time :datetime="problem.publishedAt">{{ date(problem.publishedAt) }}</time></td><td>{{ problem.timeLimitMs == null ? '—' : `${problem.timeLimitMs / 1000} 秒` }}・{{ problem.memoryLimitMb == null ? '—' : `${problem.memoryLimitMb} MiB` }}</td>
          <td>{{ problem.solverCount }}</td><td>{{ problem.favoriteCount }}</td><td><DifficultyBadge :level="problem.difficulty" /></td>
        </tr></tbody>
      </table>
    </div>
    <PageNav class="catalogue-pages" :page="page" :last="last" :link="link" />
    </template>
    <p v-if="solvedMessage" role="status">{{ solvedMessage }} <button type="button" class="editor-button" @click="loadSolved">再試行</button></p>
  </div>
</template>

<style scoped>
.catalogue-action { min-height: 40px; padding-inline: 16px; }
.catalogue-tools { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 16px; margin-bottom: 20px; }
.catalogue-tools > .tag-filter { margin-bottom: 0; }
.tags-switch { font-size: .875rem; }
.tags-hint { color: var(--color-muted); font-size: .8125rem; }
.content-table tr.solved { background: var(--color-accent-soft); }
.problem-row-tags { margin-top: 6px; }
.catalogue-pages { margin-top: 16px; }
</style>
