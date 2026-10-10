<script setup lang="ts">
import { postListSchema, type postSummarySchema } from '~~/shared/types/post'
import type { z } from 'zod'
const posts = ref<z.infer<typeof postSummarySchema>[]>([])
const cursor = ref('')
const loading = ref(true)
const error = ref('')
async function load(more = false) {
  loading.value = true; error.value = ''
  try {
    const result = postListSchema.parse(await $fetch('/api/my/posts', { query: more ? { cursor: cursor.value } : {} }))
    posts.value = more ? [...posts.value, ...result.items] : result.items; cursor.value = result.nextCursor
  } catch { error.value = '記事を取得できませんでした。もう一度お試しください。' }
  finally { loading.value = false }
}
onMounted(() => { void load() })
</script>
<template>
  <section class="draft-library">
    <header class="draft-library-heading"><h2>作成した記事</h2><NuxtLink class="editor-button primary" to="/blog/new">新規記事</NuxtLink></header>
    <p v-if="error" role="alert">{{ error }}</p><button v-if="error" class="editor-button" @click="load()">再試行</button>
    <p v-if="loading" role="status">記事を読み込んでいます…</p>
    <EmptyState v-else-if="!posts.length && !error" kind="post" title="保存した記事はまだありません。" description="「新規記事」から作成すると、ここに表示されます。" />
    <div v-if="posts.length" class="content-table-scroll">
      <table class="content-table" aria-label="作成した記事">
        <thead><tr><th scope="col">タイトル</th><th scope="col">公開状態</th><th scope="col">更新日時</th><th scope="col">操作</th></tr></thead>
        <tbody><tr v-for="post in posts" :key="post.id">
          <th scope="row">{{ post.title.trim() || '無題の記事' }}</th>
          <td>{{ post.publishedVersion ? '公開中' : '下書き' }}</td>
          <td><time :datetime="post.updatedAt">{{ new Date(post.updatedAt).toLocaleString('ja-JP', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) }}</time></td>
          <td><ContentActions :title="post.title.trim() || '無題の記事'" :edit-to="{ path: '/blog/new', query: { post: post.id } }" :view-to="`/blog/${post.id}`" :published="!!post.publishedVersion" /></td>
        </tr></tbody>
      </table>
    </div>
    <button v-if="cursor" class="editor-button" :disabled="loading" @click="load(true)">さらに読み込む</button>
  </section>
</template>
