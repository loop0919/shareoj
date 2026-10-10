<script setup lang="ts">
import { notificationMessages, type Notification } from '~~/shared/types/notification'
const { user } = useAccount()
const items = ref<Notification[]>([])
const loading = ref(true)
const error = ref('')
async function load() {
  const owner = user.value?.id
  loading.value = true
  error.value = ''
  try {
    const result = await $fetch('/api/my/notifications', { query: { history: '1' } })
    if (user.value?.id === owner) items.value = result.notifications
  } catch { error.value = 'イベントを取得できませんでした。' }
  finally { loading.value = false }
}
onMounted(load)
</script>

<template>
  <section class="draft-library" aria-labelledby="events-heading">
    <h2 id="events-heading">イベント</h2>
    <p v-if="loading" role="status">イベントを読み込んでいます…</p>
    <p v-else-if="error" role="alert">{{ error }} <button class="editor-button" @click="load">再試行</button></p>
    <EmptyState v-else-if="!items.length" kind="event" title="イベントはまだありません。" description="自分の問題がお気に入りに追加されたり、初めて正解されたりすると、ここに表示されます。" />
    <ul v-else class="event-list">
      <li v-for="item in items" :key="item.id">
        <NuxtLink :to="`/problems/${item.problemId}`">{{ item.title }}</NuxtLink>
        <p>{{ item.actor }}{{ notificationMessages[item.kind] }}</p>
        <time :datetime="item.createdAt">{{ new Date(item.createdAt).toLocaleString('ja-JP') }}</time>
      </li>
    </ul>
  </section>
</template>

<style scoped>
#events-heading { margin: 0 0 32px; }
.event-list { list-style: none; padding: 0; margin: 0; }
.event-list li { padding: 16px 0; border-bottom: 1px solid var(--color-line); overflow-wrap: anywhere; }
.event-list a { display: inline-flex; align-items: center; min-height: 44px; font-weight: 600; }
.event-list p { margin: 0 0 4px; }
.event-list time { color: var(--color-muted); font-size: .8125rem; }
</style>
