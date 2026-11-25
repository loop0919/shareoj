<script setup lang="ts">
import type { Folder } from '~~/shared/types/folders'
import { shortId } from '~~/shared/utils/short-id'
import { libraryKinds, type LibraryEntry, type LibraryKind } from '~/utils/library-entries'
// subfolders are the open folder's own folders. They lead the table, and pressing one opens it.
defineProps<{ entries: LibraryEntry[], subfolders: Folder[] }>()
const emit = defineEmits<{ open: [id: string] }>()
const kindLabels = Object.fromEntries(libraryKinds.map(kind => [kind.key, kind.label])) as Record<LibraryKind, string>
const updatedLabel = (date: string) => new Date(date).toLocaleString('ja-JP', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', timeZone: 'Asia/Tokyo' })
</script>

<template>
  <div class="content-table-scroll">
    <table class="content-table content-table--cards library-table" aria-label="問題と記事">
      <thead><tr><th scope="col">種類</th><th scope="col">タイトル</th><th scope="col">状態</th><th scope="col">更新日時</th><th scope="col">難易度</th><th scope="col">操作</th></tr></thead>
      <tbody>
        <tr v-for="folder in subfolders" :key="folder.id" class="library-folder">
          <td class="lead-cell">フォルダ</td>
          <th scope="row"><button type="button" class="library-folder-open" @click="emit('open', folder.id)">{{ folder.name }}</button></th>
          <td /><td /><td /><td />
        </tr>
        <tr v-for="entry in entries" :key="entry.key">
          <td class="lead-cell">{{ kindLabels[entry.kind] }}</td>
          <template v-if="entry.problem">
            <th scope="row">{{ entry.problem.title.trim() || '無題の問題' }}<span v-if="entry.kind === 'testing' && entry.problem.author" class="library-note">作者: {{ entry.problem.author }}</span><span v-if="entry.problem.viaContest" class="library-note">コンテスト「{{ entry.problem.viaContest.title.trim() || '無題のコンテスト' }}」の作成者として</span></th>
            <td class="line-cell" data-label="状態"><SavedProblemState :entry="entry.problem" /></td>
            <td class="corner-cell"><time :datetime="entry.problem.updatedAt">{{ updatedLabel(entry.problem.updatedAt) }}</time></td>
            <td class="lead-cell"><DifficultyBadge :level="entry.problem.difficulty" /></td>
            <td class="actions-cell"><SavedProblemActions :entry="entry.problem" :testing="entry.kind === 'testing'" /></td>
          </template>
          <template v-else-if="entry.post">
            <th scope="row">{{ entry.post.title.trim() || '無題の記事' }}</th>
            <td class="line-cell" data-label="状態">{{ entry.post.publishedVersion ? '公開中' : '下書き' }}</td>
            <td class="corner-cell"><time :datetime="entry.post.updatedAt">{{ updatedLabel(entry.post.updatedAt) }}</time></td>
            <td />
            <td class="actions-cell"><ContentActions :title="entry.post.title.trim() || '無題の記事'" :edit-to="{ path: '/blog/new', query: { post: entry.post.id } }" :view-to="`/blog/${shortId(entry.post.id)}`" :published="!!entry.post.publishedVersion" /></td>
          </template>
        </tr>
      </tbody>
    </table>
  </div>
</template>
