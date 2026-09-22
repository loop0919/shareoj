import { accountProblemSchema } from '~~/shared/types/account-problems'
import { accountError } from '~/utils/account-problems'
import { problemDraftSchema, emptyGenerators, inlineTestDataLimit, inlineTestSetLimit, type ProblemDraft } from '~~/shared/types/problem-draft'
import { draftErrors, initialEditorialMarkdown, initialProblemMarkdown, persistedDraft, testCaseError, type TestCase } from '~/utils/problem-draft'
import { uploadTestFile } from '~/utils/test-files'
import { readProblemCache, writeProblemCache, removeProblemCache } from '~/utils/problem-cache'

const saveMessages = {
  'loading': '問題を読み込んでいます…',
  'dirty': '未保存の変更があります',
  'saving': '保存しています…',
  'saved': '保存済み',
  'failed': '保存に失敗しました',
  'published': '公開しました',
  'unpublished': '非公開に戻しました',
  'load-failed': '問題を読み込めませんでした',
  'new': 'サンプルから書き始められます',
}

export function useProblemDraft() {
  const route = useRoute()
  const router = useRouter()
  const { user, refreshAccount } = useAccount()
  const cloudId = ref('')
  const cloudVersion = ref(0)
  const saving = ref(false)
  const publishing = ref(false)
  const publishedVersion = ref(0)
  const contestId = ref('')
  const manualSaveOnly = computed(() => !!publishedVersion.value || !!contestId.value)
  const publicationError = ref('')
  let cloudOwner = ''
  let disposed = false
  let inFlight: Promise<boolean> | undefined
  const saveLocation = computed(() => publishedVersion.value ? '公開中' : '非公開')
  function refreshOnFocus() { void refreshAccount().catch(() => {}) }
  const draft = reactive({ checker: null as ProblemDraft['checker'], interactor: null as ProblemDraft['interactor'], difficulty: null as number | null, title: '', markdown: initialProblemMarkdown, editorial: initialEditorialMarkdown, generators: emptyGenerators(), timeLimitMs: '2000', memoryLimitMb: '512', testCases: [] as TestCase[] })
  const ready = ref(false)
  const saveState = ref<keyof typeof saveMessages>('loading')
  const status = computed(() => saveMessages[saveState.value])
  const storageError = ref('')
  const leaveDialog = ref<HTMLDialogElement>()
  const manageDialog = ref<HTMLDialogElement>()
  const generating = ref(false)
  const confirmingDelete = ref(false)
  const deleteError = ref('')
  const errors = computed(() => draftErrors(draft))
  const leaveError = ref('')
  let deleted = false
  let resolveLeave: ((leave: boolean) => void) | undefined
  let saved = ''
  let allowAutosave = true
  let saveTimer: ReturnType<typeof setTimeout> | undefined
  const fingerprint = () => JSON.stringify(persistedDraft(draft))

  async function prepareTestFiles(id: string) {
    const encoder = new TextEncoder()
    const values = draft.testCases.flatMap(item => (['input', 'output'] as const).map(key => ({ item, key, text: item[key], size: encoder.encode(item[key]).length })))
      .filter(value => !(value.item[`${value.key}File`] && !value.item[`_${value.key}Dirty`]))
    let inline = values.reduce((sum, value) => sum + value.size, 0)
    const upload = new Set(values.filter(value => value.size > inlineTestDataLimit))
    for (const value of [...values].sort((a, b) => b.size - a.size)) {
      if (inline <= inlineTestSetLimit) break
      if (value.size && !upload.has(value)) upload.add(value)
      inline -= value.size
    }
    for (const value of upload) {
      const file = await uploadTestFile(id, value.text)
      if (!draft.testCases.includes(value.item) || value.item[value.key] !== value.text) return false
      value.item[`${value.key}File`] = file
      value.item[`_${value.key}Dirty`] = false
    }
    return true
  }

  async function saveDraft(manual = false, updateLocation = true): Promise<boolean> {
    if (disposed || deleted || publishing.value || confirmingDelete.value || !ready.value || (!manual && !allowAutosave)) return false
    const testError = testCaseError(draft.testCases)
    if (testError) { saveState.value = 'dirty'; storageError.value = testError; return false }
    if (!manual && fingerprint() === saved) return true
    if (!manual && manualSaveOnly.value) { storageError.value = '変更を反映するには保存ボタンを押してください。'; return false }
    clearTimeout(saveTimer)
    if (inFlight) {
      if (!await inFlight) return false
      if (fingerprint() === saved) return true
      return saveDraft(manual, updateLocation)
    }
    const version = cloudVersion.value
    cloudId.value ||= crypto.randomUUID()
    const id = cloudId.value
    saving.value = true
    saveState.value = 'saving'
    inFlight = (async () => {
      try {
        const account = await refreshAccount()
        if (!account || account.id !== cloudOwner) throw { statusCode: 401 }
        if (!await prepareTestFiles(id)) return false
        const snapshot = fingerprint()
        let result
        try {
          result = accountProblemSchema.parse(await $fetch(`/api/my/problems/${id}`, { method: 'PUT', body: { version, draft: JSON.parse(snapshot) } }))
        } catch (error) {
          // A response can be lost after a successful commit. Recognize that exact write.
          const current = await $fetch(`/api/my/problems/${id}`).catch(() => null)
          const parsed = accountProblemSchema.safeParse(current)
          if (!parsed.success || parsed.data.version !== version + 1 || JSON.stringify(parsed.data.draft) !== JSON.stringify(problemDraftSchema.parse(JSON.parse(snapshot)))) throw error
          result = parsed.data
        }
        if (result.id !== id) throw new Error('Mismatched problem')
        writeProblemCache(cloudOwner, result)
        cloudVersion.value = result.version
        publishedVersion.value = result.publishedVersion
        contestId.value = result.contestId
        saved = snapshot
        allowAutosave = true
        storageError.value = ''
        // Reload must reopen this draft before we announce that saving is complete.
        if (updateLocation && route.query.problem !== id) await router.replace({ path: '/problems/new', query: { problem: id } })
        saveState.value = fingerprint() === saved ? 'saved' : 'dirty'
        return true
      } catch (error) {
        allowAutosave = false
        saveState.value = 'failed'
        storageError.value = accountError(error)
        return false
      } finally { saving.value = false }
    })()
    try { return await inFlight }
    finally { inFlight = undefined }
  }

  async function publishProblem(publish: boolean) {
    if (publishing.value || saving.value || generating.value) return
    publicationError.value = ''
    if (publish && (!draft.title.trim() || !draft.markdown.trim())) { publicationError.value = '公開するにはタイトルと本文を入力してください。'; return }
    if (publish && errors.value.checker) { publicationError.value = errors.value.checker; return }
    if (!await saveDraft(true)) return
    if (!window.confirm(publish ? '現在の内容を公開しますか？誰でも閲覧できるようになります。' : 'この問題を非公開に戻しますか？')) return
    clearTimeout(saveTimer)
    publishing.value = true
    try {
      const result = accountProblemSchema.parse(await $fetch(`/api/my/problems/${cloudId.value}/publication`, { method: 'PUT', body: { version: cloudVersion.value, publish } }))
      cloudVersion.value = result.version
      publishedVersion.value = result.publishedVersion
      contestId.value = result.contestId
      writeProblemCache(cloudOwner, result)
      saveState.value = publish ? 'published' : 'unpublished'
    } catch (error) { publicationError.value = accountError(error) }
    finally { publishing.value = false }
  }

  function saveWithShortcut(event: KeyboardEvent) {
    if (event.isComposing || event.altKey || event.shiftKey || !(event.ctrlKey || event.metaKey) || event.key.toLowerCase() !== 's') return
    event.preventDefault()
    if (event.repeat || resolveLeave || confirmingDelete.value) return
    saveDraft(true)
  }

  function flushBeforeLeave(event: BeforeUnloadEvent) {
    if (ready.value && fingerprint() !== saved) {
      event.preventDefault()
      event.returnValue = ''
    }
  }

  onMounted(async () => {
    window.addEventListener('focus', refreshOnFocus)
    try { await refreshAccount() } catch { user.value = null }
    if (disposed) return
    if (!user.value) {
      await navigateTo({ path: '/login', query: { next: route.fullPath } }, { replace: true })
      return
    }
    cloudOwner = user.value.id
    if (typeof route.query.problem === 'string') {
      cloudId.value = route.query.problem
      try {
        if (!user.value) throw { statusCode: 401 }
        cloudOwner = user.value.id
        const cached = readProblemCache(cloudOwner, cloudId.value)
        if (cached) { Object.assign(draft, cached.draft) }
        const entry = accountProblemSchema.parse(await $fetch(`/api/my/problems/${encodeURIComponent(cloudId.value)}`))
        if (disposed) return
        if (entry.id !== cloudId.value) throw new Error('Mismatched problem')
        writeProblemCache(cloudOwner, entry)
        Object.assign(draft, entry.draft)
        if (Number(draft.memoryLimitMb) > 512) draft.memoryLimitMb = '512'
        cloudVersion.value = entry.version
        publishedVersion.value = entry.publishedVersion
        contestId.value = entry.contestId
        saveState.value = 'saved'
      } catch (error) {
        removeProblemCache(cloudOwner, cloudId.value)
        Object.assign(draft, { checker: null, interactor: null, difficulty: null as number | null, title: '', markdown: initialProblemMarkdown, editorial: initialEditorialMarkdown, generators: emptyGenerators(), timeLimitMs: '2000', memoryLimitMb: '512', testCases: [] })
        saveState.value = 'load-failed'
        storageError.value = accountError(error)
        return
      }
    } else {
      saveState.value = 'new'
    }
    saved = fingerprint()
    // Flush restoration watchers before enabling automatic writes.
    nextTick(() => { if (!disposed) ready.value = true })
    window.addEventListener('beforeunload', flushBeforeLeave)
    window.addEventListener('keydown', saveWithShortcut)
  })

  watch(draft, () => {
    if (!ready.value) return
    saveState.value = 'dirty'
    clearTimeout(saveTimer)
    if (!manualSaveOnly.value) saveTimer = setTimeout(() => saveDraft(), 600)
  })

  onBeforeRouteLeave(() => {
    if (saving.value || publishing.value) return false
    if (!ready.value || fingerprint() === saved) return true
    if (resolveLeave) return false
    clearTimeout(saveTimer)
    leaveError.value = ''
    return new Promise<boolean>(resolve => {
      resolveLeave = resolve
      leaveDialog.value?.showModal()
    })
  })
  async function finishLeave(choice: 'stay' | 'discard' | 'save') {
    if (choice === 'save' && !await saveDraft(true, false)) {
      leaveError.value = '保存できませんでした。編集を続けるか、保存せずに移動してください。'
      return
    }
    leaveDialog.value?.close()
    const resolve = resolveLeave
    resolveLeave = undefined
    resolve?.(choice !== 'stay')
    if (choice === 'stay' && fingerprint() !== saved && !manualSaveOnly.value) saveTimer = setTimeout(() => saveDraft(), 600)
  }
  onBeforeUnmount(() => {
    disposed = true
    window.removeEventListener('focus', refreshOnFocus)
    resolveLeave?.(false)
    resolveLeave = undefined
    clearTimeout(saveTimer)
    window.removeEventListener('beforeunload', flushBeforeLeave)
    window.removeEventListener('keydown', saveWithShortcut)
  })

  function openDeleteConfirmation() {
    clearTimeout(saveTimer)
    if (saving.value || publishing.value || generating.value) return
    confirmingDelete.value = true
    deleteError.value = ''
    manageDialog.value?.showModal()
  }
  function closeDeleteConfirmation() {
    manageDialog.value?.close()
    confirmingDelete.value = false
    if (!deleted && fingerprint() !== saved && !manualSaveOnly.value) saveTimer = setTimeout(() => saveDraft(), 600)
  }
  async function removeProblem() {
    clearTimeout(saveTimer)
    try {
      if (cloudId.value && cloudVersion.value > 0) {
        await $fetch(`/api/my/problems/${cloudId.value}`, { method: 'DELETE', query: { version: cloudVersion.value } })
      }
      removeProblemCache(cloudOwner, cloudId.value)
      deleted = true
      saved = fingerprint()
      manageDialog.value?.close()
      await router.replace('/my?tab=problems')
    } catch (error) {
      deleteError.value = accountError(error)
    }
  }

  return {
    user, draft, ready, cloudId, cloudVersion, saving, publishing, publishedVersion, contestId, manualSaveOnly,
    publicationError, saveLocation, saveState, status, storageError, leaveDialog, leaveError,
    manageDialog, generating, deleteError, errors, saveDraft, publishProblem, finishLeave,
    openDeleteConfirmation, closeDeleteConfirmation, removeProblem,
  }
}
