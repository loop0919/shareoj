<script setup lang="ts">
import { loginDestination } from '~~/shared/utils/login-destination'
const route = useRoute()
import { accountServices, accountsSchema, deletedHandle, profileResultSchema, type Profile } from '~~/shared/types/profile'
import { profileError } from '~/utils/profile'
// Settings show one part per tab; onboarding shows both. Every save sends the whole profile.
const props = defineProps<{ initial: Profile | null, onboarding?: boolean, part?: 'identity' | 'services' }>()
const { user, profile, refreshAccount } = useAccount()
const owner = user.value?.id
const handle = ref(props.initial?.handle ?? '')
const accounts = reactive(accountsSchema.parse(props.initial?.accounts ?? {}))
const avatar = ref(props.initial?.avatar ?? '')
const version = ref(props.initial?.version ?? 0)
const saved = ref(false)
watch([handle, avatar, accounts], () => { saved.value = false })
const saving = ref(false)
const processing = ref(false)
const error = ref('')
const imageError = ref('')
const normalizedHandle = computed(() => handle.value.trim().toLowerCase())
const validHandle = computed(() => /^[a-z][a-z0-9_]{2,19}$/.test(normalizedHandle.value))
async function chooseAvatar(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  imageError.value = ''
  if (!['image/png', 'image/jpeg', 'image/webp'].includes(file.type) || file.size > 5 * 1024 * 1024) {
    imageError.value = 'PNG・JPEG・WebPの画像を5MB以下で選んでください。'; return
  }
  processing.value = true
  try {
    const bitmap = await createImageBitmap(file)
    try {
      const canvas = document.createElement('canvas')
      canvas.width = 128; canvas.height = 128
      const context = canvas.getContext('2d')
      if (!context) throw new Error('Canvas unavailable')
      const size = Math.min(bitmap.width, bitmap.height)
      context.drawImage(bitmap, (bitmap.width - size) / 2, (bitmap.height - size) / 2, size, size, 0, 0, 128, 128)
      avatar.value = canvas.toDataURL('image/png')
    } finally { bitmap.close() }
  } catch { imageError.value = '画像を読み込めませんでした。別の画像を選んでください。' }
  finally { processing.value = false }
}
async function save() {
  if (saving.value || processing.value) return
  error.value = ''
  saved.value = false
  if (!validHandle.value) { error.value = 'ユーザーIDは英字で始まる3〜20文字の英小文字・数字・アンダースコアで入力してください。'; return }
  if (deletedHandle.test(normalizedHandle.value)) { error.value = 'このユーザーIDは使えません。別のIDを入力してください。'; return }
  saving.value = true
  try {
    const account = await refreshAccount()
    if (!account || account.id !== owner) throw { statusCode: 401 }
    const result = profileResultSchema.parse(await $fetch('/api/my/profile', { method: 'PUT', body: { handle: normalizedHandle.value, avatar: avatar.value, accounts, version: version.value } }))
    if (!result.profile) throw new Error('Missing profile')
    profile.value = result.profile
    version.value = result.profile.version
    if (props.onboarding) await navigateTo(loginDestination(route.query.next))
    else saved.value = true
  } catch (e) { error.value = profileError(e) }
  finally { saving.value = false }
}
</script>
<template>
  <form class="profile-form" @submit.prevent="save">
    <fieldset :disabled="saving || processing">
      <legend class="sr-only">プロフィール</legend>
      <template v-if="part !== 'services'">
        <div class="avatar-field">
          <UserAvatar :handle="normalizedHandle" :avatar="avatar" :size="88" />
          <div>
            <label for="profile-icon">アイコン（任意）</label>
            <input id="profile-icon" type="file" accept="image/png,image/jpeg,image/webp" @change="chooseAvatar">
            <p class="hint">PNG・JPEG・WebP、5MBまで。中央を正方形に切り抜きます。</p>
            <button v-if="avatar" class="editor-button" type="button" @click="avatar = ''">アイコンを削除</button>
          </div>
        </div>
        <p v-if="imageError" class="editor-error" role="alert">{{ imageError }}</p>
        <label for="profile-handle">ユーザーID</label>
        <input id="profile-handle" v-model="handle" class="handle-input" type="text" minlength="3" maxlength="20" autocomplete="username" autocapitalize="none" spellcheck="false" required aria-describedby="handle-help">
        <p id="handle-help" class="hint">英字で始まる3〜20文字。英小文字・数字・_ が使えます。大文字は小文字になります。</p>
      </template>
      <template v-if="part !== 'identity'">
        <h2 v-if="!part">外部アカウント（任意）</h2>
        <p class="hint">入力した ID はユーザーページで公開されます。AtCoder・Codeforces はレーティングに応じた色で表示します。</p>
        <div v-for="service in accountServices" :key="service.key" class="account-field">
          <label :for="`account-${service.key}`">{{ service.label }} ID</label>
          <input :id="`account-${service.key}`" v-model="accounts[service.key]" class="handle-input" type="text" :pattern="service.pattern" :maxlength="service.max" autocomplete="off" autocapitalize="none" spellcheck="false" :aria-describedby="service.key === 'yukicoder' ? 'yukicoder-help' : undefined">
          <p v-if="service.key === 'yukicoder'" id="yukicoder-help" class="hint">プロフィール URL（yukicoder.me/users/123）の数値部分を入力してください。ユーザーページには取得したユーザー名を表示します。</p>
        </div>
      </template>
      <p v-if="error" class="editor-error" role="alert">{{ error }}</p>
      <button class="editor-button primary" type="submit" :aria-busy="saving">{{ saving ? '保存中…' : onboarding ? '登録してはじめる' : '変更を保存' }}</button>
      <span v-if="saved" class="saved-status" role="status">保存しました</span>
    </fieldset>
    <p v-if="processing" role="status">画像を準備しています…</p>
  </form>
</template>
<style scoped>
.profile-form { max-width: 36rem; }
fieldset { border: 0; padding: 0; margin: 0; min-width: 0; }
label { display: block; margin-bottom: 8px; font-weight: 600; }
.avatar-field { display: flex; gap: 24px; align-items: center; margin-bottom: 32px; }
.avatar-field > div { min-width: 0; }
input[type=file] { max-width: 100%; font: inherit; font-size: .85rem; }
.handle-input { width: 100%; border: 1px solid var(--color-line); border-radius: 4px; min-height: 44px; padding: 8px 12px; font: inherit; }
.hint { margin: 8px 0 24px; color: var(--color-muted); font-size: .85rem; }
.account-field { margin-bottom: 20px; }
.saved-status { margin-left: 16px; color: var(--color-muted); font-size: .875rem; }
.sr-only { position: absolute; width: 1px; height: 1px; padding: 0; overflow: hidden; clip-path: inset(50%); }
@media(max-width: 480px) { .avatar-field { align-items: flex-start; flex-direction: column; gap: 16px; } }
</style>
