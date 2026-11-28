<script setup lang="ts">
import { clampPage, lastPage, pageOffset } from '~/utils/pagination'
import type { PageSize } from '~/composables/usePageSize'
import { conditionChips, conditionQuery } from '~/utils/problem-conditions'
import { shortId } from '~~/shared/utils/short-id'
import { japanDate } from '~/utils/japan-time'
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
// Rendering depends on the page size cookie.
useResponseHeader('Vary').value = 'Cookie'
// The sort and the filters are in the URL too. The page stays mounted when they change, so the panel stays as it is.
const { conditions, change, clear } = useProblemConditions()
const conditionsOpen = ref(false)
const tag = computed(() => conditions.value.tag)
const contest = computed(() => conditions.value.contest)
// Problem tags can give away the solution, so each visit starts with them hidden until the viewer turns them on. The
// list of another tag is another visit.
const showTags = ref(false)
watch(tag, () => { showTags.value = false })
// The page is in the URL and the page size in a cookie, so the server renders the rows the viewer sees (ADR 0028).
const { requested, link, resize } = usePageQuery()
const size = usePageSize('problems')
const offset = computed(() => pageOffset(requested.value, size.value))
// A fixed key, so useFetch reloads once a move has set both the offset and the size: a key made of them is watched
// synchronously and would load the old page at the new size in between. The route catching up with a page already
// requested leaves the offset, and so the list, as it is. New conditions come with the first page in one route, so they
// load once too. The server applies solved only for a session.
const { data, error, status, refresh } = await useFetch('/api/problems', { key: 'problem-list', query: computed(() => ({ ...conditionQuery(conditions.value), offset: offset.value, limit: size.value })) })
if (error.value?.statusCode === 400) throw createError({ statusCode: 404, statusMessage: 'Tag not found', fatal: true })
if (error.value || !data.value) throw createError({ statusCode: 502, statusMessage: 'Problem service unavailable', fatal: true })
// The chips and the panel show the solved filter the list really applied, never one a signed-out viewer has in the URL.
const solvedApplied = computed(() => Boolean(data.value?.solved))
const chips = computed(() => conditionChips(conditions.value, solvedApplied.value))
// Row tags keep the other conditions.
const tagKeep = computed(() => conditionQuery({ ...conditions.value, tag: '' }))
// A tag alone, or nothing, keeps its own message; anything else may be what emptied the list, so it offers a way out.
const narrowed = computed(() => chips.value.some(chip => chip.key !== 'tag'))
const emptyTitle = computed(() => narrowed.value ? '条件に合う問題はありません。' : tag.value ? `「${tag.value}」のタグが付いた問題はまだありません。` : '公開された問題はまだありません。')
const last = computed(() => lastPage(data.value?.total ?? 0, size.value))
const page = computed(() => clampPage(requested.value, last.value))
// A page past the end is replaced by the last page, whether the server renders it or the browser moves to it.
const toLastPage = () => data.value && requested.value > last.value ? navigateTo(link(last.value), { replace: true }) : undefined
await toLastPage()
watch(data, () => { void toLastPage() })
const resizeProblems = (to: PageSize) => resize(size, page.value, to)
// The canonical is the page on screen, by tag and contest only: the order and the other filters are the viewer's own.
// The first page leaves out page.
const path = computed(() => {
  const query = new URLSearchParams({ ...tag.value ? { tag: tag.value } : {}, ...contest.value ? { contest: contest.value } : {}, ...page.value > 1 ? { page: String(page.value) } : {} })
  return query.size ? `/problems?${query}` : '/problems'
})
const canonical = computed(() => new URL(path.value, config.public.siteUrl).href)
const heading = computed(() => tag.value ? `タグ「${tag.value}」の問題` : '問題')
const description = computed(() => tag.value ? `ShareOJ で「${tag.value}」のタグが付いたプログラミング問題の一覧。` : 'ShareOJ のプログラミング問題一覧。ユーザーが作成・公開した問題に挑戦し、コードを提出して自動採点できます。')
const title = computed(() => `${heading.value} | ShareOJ`)
useSeoMeta({ title, description, ogDescription: description, ogTitle: title, ogUrl: canonical, ogType: 'website' })
useHead({ link: [{ rel: 'canonical', href: canonical }] })
useSharePreview({ type: 'website', title: heading, path, description })
</script>
<template>
  <div class="catalogue">
    <header class="catalogue-heading"><h1>問題</h1><NuxtLink class="editor-button primary" to="/problems/post">問題投稿</NuxtLink></header>
    <!-- The tag switch stays outside the panel, at the end of the button's row. -->
    <ProblemConditions v-model:open="conditionsOpen" class="catalogue-conditions" :conditions="conditions" :solved-applied="solvedApplied" :signed-in="Boolean(user)" :show-tags="showTags" @change="change" @clear="clear">
      <span class="catalogue-tags">
        <InlineSwitch v-model="showTags" class="tags-switch" :describedby="showTags ? undefined : 'problem-tags-hint'">タグを表示</InlineSwitch>
        <span v-if="!showTags" id="problem-tags-hint" class="tags-hint">解法のヒントになることがあります。</span>
      </span>
    </ProblemConditions>
    <!-- A page that fails after the first one hides the rows it would replace. -->
    <p v-if="error" role="alert">一覧を取得できませんでした。もう一度お試しください。 <button type="button" class="editor-button" @click="refresh()">再試行</button></p>
    <EmptyState v-else-if="!data?.total" kind="problem" :title="emptyTitle">
      <template v-if="narrowed" #default><button type="button" class="editor-button" @click="clear">条件をすべて外す</button></template>
    </EmptyState>
    <template v-else>
    <ListToolbar :total="data.total" :page="page" :size="size" unit="問" :link="link" @update:size="resizeProblems" />
    <div class="content-table-scroll" tabindex="0" role="region" aria-label="問題一覧" :aria-busy="status === 'pending'">
      <table class="content-table content-table--cards">
        <thead><tr><th scope="col">公開日</th><th scope="col">タイトル</th><th scope="col">作成者</th><th scope="col"><abbr title="実行時間制限 / メモリ制限">TL / ML</abbr></th><th scope="col">正解者数</th><th scope="col"><abbr title="お気に入り数">Fav</abbr></th><th scope="col">難易度</th></tr></thead>
        <tbody><tr v-for="problem in data.items" :key="problem.id" :class="{ solved: solved.has(problem.id) }">
          <td class="corner-cell"><time :datetime="problem.publishedAt">{{ japanDate(problem.publishedAt) }}</time></td>
          <th scope="row"><NuxtLink :to="`/problems/${shortId(problem.id)}`" :aria-label="solved.has(problem.id) ? `${problem.title}（AC 済み）` : undefined">{{ problem.title }}</NuxtLink><TagLinks v-if="showTags && problem.tags.length" class="problem-row-tags" :tags="problem.tags" to="/problems" :keep="tagKeep" /></th>
          <td class="line-cell" data-label="作成者"><UserLink :handle="problem.author" /></td><td class="stat-cell wide-cell" data-label="TL / ML">{{ problem.timeLimitMs == null ? '—' : `${problem.timeLimitMs / 1000} 秒` }}・{{ problem.memoryLimitMb == null ? '—' : `${problem.memoryLimitMb} MiB` }}</td>
          <td class="stat-cell" data-label="正解者数">{{ problem.solverCount }}</td><td class="stat-cell" data-label="Fav">{{ problem.favoriteCount }}</td><td class="lead-cell"><DifficultyBadge :level="problem.difficulty" /></td>
        </tr></tbody>
      </table>
    </div>
    <PageNav class="catalogue-pages" :page="page" :last="last" :link="link" />
    </template>
    <p v-if="solvedMessage" role="status">{{ solvedMessage }} <button type="button" class="editor-button" @click="loadSolved">再試行</button></p>
  </div>
</template>

<style scoped>
.catalogue-conditions { margin-bottom: 20px; }
.catalogue-tags { display: flex; flex-wrap: wrap; align-items: center; gap: 0 12px; margin-left: auto; }
@media (max-width: 39.999rem) { .catalogue-tags { margin-left: 0; } }
.tags-switch { font-size: .875rem; }
.tags-hint { color: var(--color-muted); font-size: .8125rem; }
.content-table tr.solved { background: var(--color-accent-soft); }
.problem-row-tags { margin-top: 6px; }
.catalogue-pages { margin-top: 16px; }
</style>
