<script setup lang="ts">
import { contentImageError } from '~/utils/content-image'

defineProps<{ source: string, disabled?: boolean }>()
const emit = defineEmits<{ insert: [snippet: string], close: [], upload: [] }>()
const offset = ref(0)
const busy = ref(false)
const message = ref('')
const { data, error, refresh } = useFetch<{ items: { id: string, size: number, used: boolean }[], usedBytes: number, hasMore: boolean }>('/api/my/images', { query: { offset } })
async function remove(id: string) {
  if (!window.confirm('この画像をストレージから削除します。元に戻せません。削除しますか？')) return
  busy.value = true
  message.value = ''
  try {
    await $fetch(`/api/my/images/${id}`, { method: 'DELETE' })
    if (data.value?.items.length === 1 && offset.value > 0) offset.value = Math.max(0, offset.value - 50)
    await refresh()
  } catch (error) { message.value = contentImageError(error) }
  finally { busy.value = false }
}
</script>

<template>
  <section class="image-library" aria-label="保存した画像">
    <div class="image-library-heading">
      <strong>保存した画像</strong>
      <button type="button" class="editor-button image-add" aria-label="画像を追加" :disabled="disabled || busy" @click="emit('upload')"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 5v14M5 12h14" /></svg>追加</button>
      <span v-if="data" class="image-library-muted">使用量 {{ (data.usedBytes / 1024 / 1024).toFixed(1) }} MB</span>
      <button type="button" class="editor-button image-icon-button image-close" aria-label="閉じる" title="閉じる" @click="emit('close')"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m6 6 12 12M18 6 6 18" /></svg></button>
    </div>
    <p class="image-library-muted">画像は編集欄へのドロップ・貼り付けでも追加できます（1枚10MBまで・自動縮小）。</p>
    <p v-if="error || message" role="alert">{{ message || '画像一覧を取得できませんでした。' }}</p>
    <div class="image-library-items">
      <div v-for="item in data?.items" :key="item.id" class="image-library-item">
        <button type="button" class="editor-button image-thumbnail" aria-label="画像を本文に挿入" title="クリックして本文に挿入" :disabled="disabled || busy" @click="emit('insert', `![画像の説明](/api/images/${item.id})`)"><img :src="`/api/images/${item.id}`" alt="保存した画像のプレビュー" loading="lazy"></button>
        <span class="image-library-muted">{{ Math.ceil(item.size / 1024) }} KB</span>
        <button type="button" class="editor-button image-icon-button" aria-label="削除" title="画像を削除" :disabled="disabled || busy || item.used || source.includes(`/api/images/${item.id}`)" @click="remove(item.id)"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7M14 10v7" /></svg></button>
      </div>
    </div>
    <p v-if="data && !data.items.length">保存した画像はありません。</p>
    <button v-if="offset" type="button" class="editor-button" :disabled="busy" @click="offset -= 50">前へ</button>
    <button v-if="data?.hasMore" type="button" class="editor-button" :disabled="busy" @click="offset += 50">次へ</button>
  </section>
</template>

<style scoped>
.image-library { padding: 12px 16px; border-bottom: 1px solid var(--color-line); overflow: auto; max-height: 300px; font-size: .8rem; }
.image-library-heading { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.image-library-muted { color: var(--color-muted); }
.image-add, .image-icon-button { display: inline-flex; align-items: center; justify-content: center; gap: 4px; }
.image-icon-button { width: var(--editor-control-size); padding: 4px; border-color: transparent; background: transparent; }
.image-close { margin-left: auto; }
.image-thumbnail { width: 150px; height: 90px; padding: 0; border-color: transparent; background: transparent; }
.image-library-items { display: flex; gap: 12px; flex-wrap: wrap; }
.image-library-item { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; width: 150px; }
.image-library-item img { display: block; width: 100%; height: 100%; object-fit: contain; }
</style>
