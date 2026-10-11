export function useMarkdownEditor() {
  const mode = ref<'edit' | 'split' | 'preview'>('split')
  const workspace = ref<HTMLElement>()
  const splitPercent = ref(50)
  const resizing = ref(false)
  let dragStartX = 0
  let dragStartPercent = 50
  function setSplit(value: number) {
    splitPercent.value = Math.min(70, Math.max(30, value))
  }
  function startResize(event: PointerEvent) {
    if (event.button !== 0 || !event.isPrimary) return
    dragStartX = event.clientX
    dragStartPercent = splitPercent.value
    resizing.value = true
    ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
    event.preventDefault()
  }
  function moveResize(event: PointerEvent) {
    if (!resizing.value || !workspace.value) return
    const width = workspace.value.getBoundingClientRect().width - 8
    if (width > 0) setSplit(dragStartPercent + (event.clientX - dragStartX) / width * 100)
  }
  function stopResize(event: PointerEvent) {
    resizing.value = false
    const handle = event.currentTarget as HTMLElement
    if (handle.hasPointerCapture(event.pointerId)) handle.releasePointerCapture(event.pointerId)
  }
  function resizeWithKeyboard(event: KeyboardEvent) {
    const values: Record<string, number> = { ArrowLeft: splitPercent.value - 2, ArrowRight: splitPercent.value + 2, Home: 30, End: 70, Enter: 50 }
    const value = values[event.key]
    if (value === undefined) return
    event.preventDefault()
    setSplit(value)
  }

  const editor = ref<{
    insertSnippet: (snippet: string) => void, toggleImages: () => void, showImages: boolean, uploading: boolean, requestMeasure: () => void, focus: () => void,
    scroller: () => HTMLElement | undefined, lineCount: () => number, lineAtHeight: (height: number) => number, heightAtLine: (line: number) => number,
  }>()
  const syncSource = () => editor.value?.requestMeasure()
  async function insertSnippet(snippet: string) {
    if (mode.value === 'preview') mode.value = 'edit'
    await nextTick()
    editor.value?.insertSnippet(snippet)
  }
  watch(mode, () => nextTick(syncSource))

  // Scroll sync maps each pane's position to a fractional source line. The followed point moves from
  // the top of the viewport to its bottom as a pane scrolls, so both panes reach their ends together.
  const preview = ref<HTMLElement>()
  const { settings } = useEditorSettings('markdown')
  type Pane = 'editor' | 'preview'
  type Anchor = [line: number, height: number]
  let programmatic: { element: HTMLElement, top: number } | undefined
  let followFrame = 0
  const scrollerOf = (pane: Pane) => pane === 'editor' ? editor.value?.scroller() : preview.value
  function previewAnchors(element: HTMLElement, lines: number) {
    const origin = element.getBoundingClientRect().top - element.scrollTop
    const points: Anchor[] = []
    for (const block of element.querySelectorAll<HTMLElement>('[data-source-line]')) {
      if (!block.getClientRects().length) continue
      const box = block.getBoundingClientRect()
      points.push([Number(block.dataset.sourceLine), box.top - origin], [Number(block.dataset.sourceEnd), box.bottom - origin])
    }
    points.sort((a, b) => a[0] - b[0] || a[1] - b[1])
    const anchors: Anchor[] = [[0, 0]]
    for (const point of points) {
      const last = anchors.at(-1)!
      if (point[0] > last[0] && point[0] < lines && point[1] >= last[1]) anchors.push(point)
    }
    anchors.push([lines, element.scrollHeight])
    return anchors
  }
  function interpolate(anchors: Anchor[], value: number, from: 0 | 1) {
    const to = from === 0 ? 1 : 0
    const next = anchors.findIndex(anchor => anchor[from] > value)
    if (next <= 0) return anchors.at(next)![to]
    const [a, b] = [anchors[next - 1]!, anchors[next]!]
    return a[to] + (b[to] - a[to]) * (value - a[from]) / (b[from] - a[from])
  }
  function lineIn(pane: Pane) {
    const source = editor.value
    const element = scrollerOf(pane)
    if (!source || !element?.clientHeight) return
    const range = element.scrollHeight - element.clientHeight
    const height = range > 0 ? element.scrollTop * element.scrollHeight / range : 0
    return pane === 'editor' ? source.lineAtHeight(height) : interpolate(previewAnchors(element, source.lineCount()), height, 1)
  }
  function scrollToLine(pane: Pane, line: number) {
    const source = editor.value
    const element = scrollerOf(pane)
    if (!source || !element?.clientHeight) return
    const height = pane === 'editor' ? source.heightAtLine(line) : interpolate(previewAnchors(element, source.lineCount()), line, 0)
    const before = element.scrollTop
    element.scrollTop = height * (element.scrollHeight - element.clientHeight) / element.scrollHeight
    if (element.scrollTop !== before) programmatic = { element, top: element.scrollTop }
  }
  function follow(from: Pane) {
    if (!settings.value.scrollSync || mode.value !== 'split') return
    const line = lineIn(from)
    if (line !== undefined) scrollToLine(from === 'editor' ? 'preview' : 'editor', line)
  }
  function scrolled(from: Pane) {
    const element = scrollerOf(from)
    // Ignore the scroll event caused by following the other pane.
    if (element && programmatic?.element === element && Math.abs(element.scrollTop - programmatic.top) < 1) {
      programmatic = undefined
      return
    }
    follow(from)
  }
  // Re-align the preview after its content or size changes, such as while typing or resizing panes.
  function followEditorLater() {
    cancelAnimationFrame(followFrame)
    followFrame = requestAnimationFrame(() => follow('editor'))
  }
  watch(preview, (element, _, onCleanup) => {
    if (!element) return
    const resized = new ResizeObserver(followEditorLater)
    const changed = new MutationObserver(followEditorLater)
    resized.observe(element)
    changed.observe(element, { childList: true, subtree: true, characterData: true })
    element.addEventListener('load', followEditorLater, true)
    onCleanup(() => {
      resized.disconnect()
      changed.disconnect()
      element.removeEventListener('load', followEditorLater, true)
    })
  })
  watch(() => settings.value.scrollSync, followEditorLater)
  // Switching views keeps the line that was on screen; measure it before the hidden pane leaves the layout.
  watch(mode, (now, before) => {
    if (!settings.value.scrollSync) return
    const from: Pane = before === 'preview' ? 'preview' : 'editor'
    const to: Pane = now === 'edit' ? 'editor' : now === 'preview' ? 'preview' : from === 'editor' ? 'preview' : 'editor'
    const line = from === to ? undefined : lineIn(from)
    if (line !== undefined) nextTick(() => requestAnimationFrame(() => scrollToLine(to, line)))
  })
  onBeforeUnmount(() => cancelAnimationFrame(followFrame))

  onMounted(() => {
    if (window.matchMedia('(max-width: 59.999rem)').matches) mode.value = 'edit'
  })
  return {
    mode, workspace, splitPercent, resizing, setSplit, startResize, moveResize, stopResize, resizeWithKeyboard, editor, syncSource, insertSnippet,
    preview, editorScrolled: () => scrolled('editor'), previewScrolled: () => scrolled('preview'),
  }
}
