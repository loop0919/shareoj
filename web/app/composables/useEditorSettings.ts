export function useEditorSettings(kind: 'code' | 'markdown', language: MaybeRefOrGetter<string> = 'cpp') {
  type Settings = { style: 'space' | 'tab', width: number, scrollSync: boolean }
  const scope = computed(() => kind === 'markdown' ? 'markdown' : `code.${toValue(language)}`)
  const allSettings = useState<Record<string, Settings>>('editor-settings-by-language', () => ({}))
  const loaded = useState<Record<string, boolean>>('editor-settings-loaded', () => ({}))
  const settings = computed(() => allSettings.value[scope.value] ??= {
    style: kind === 'code' && toValue(language) === 'go' ? 'tab' : 'space',
    width: kind === 'markdown' || ['javascript', 'typescript', 'ruby', 'nim', 'haskell'].includes(toValue(language)) ? 2 : 4,
    scrollSync: true,
  })
  function loadSettings() {
    if (loaded.value[scope.value]) return
    loaded.value[scope.value] = true
    try {
      const legacy = kind === 'code' && toValue(language) === 'cpp'
        ? localStorage.getItem('openoj.editor-settings.code') ?? localStorage.getItem('openoj.editor-settings') : null
      const saved = JSON.parse(localStorage.getItem(`openoj.editor-settings.${scope.value}`) ?? legacy ?? 'null')
      if (saved && ['space', 'tab'].includes(saved.style) && Number.isInteger(saved.width) && saved.width >= 1 && saved.width <= 8) {
        allSettings.value[scope.value] = { style: saved.style, width: saved.width, scrollSync: saved.scrollSync !== false }
      }
    } catch { /* Keep editing available without browser storage. */ }
  }
  onMounted(() => {
    loadSettings()
    watch(scope, loadSettings, { flush: 'sync' })
  })
  function saveSettings() {
    try { localStorage.setItem(`openoj.editor-settings.${scope.value}`, JSON.stringify(settings.value)) }
    catch { /* Settings still apply for this session. */ }
  }
  const indentation = computed(() => settings.value.style === 'tab' ? '\t' : ' '.repeat(settings.value.width))
  return { settings, indentation, saveSettings }
}
