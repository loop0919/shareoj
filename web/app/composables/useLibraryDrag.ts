import { onBeforeUnmount, onMounted, ref, type Ref } from 'vue'
import type { FolderRef } from '~~/shared/types/folders'

// Problems and posts dragged from the table, or a folder dragged in the folder list.
export type LibraryDrag = { kind: 'items', refs: FolderRef[] } | { kind: 'folder', id: string }

// What is being dragged, shared by the table and the folder list so the list can mark where it may go. The drag carries
// a line of text for other apps: how many items move, or the folder's name, which the caller passes since the drag
// holds only its id. More than one item shows that line in place of the row the pointer took.
export function useLibraryDrag(): { dragging: Ref<LibraryDrag | null>, start(drag: LibraryDrag, event: DragEvent, name?: string): void, end(): void } {
  const dragging = useState<LibraryDrag | null>('library-drag', () => null)
  function start(drag: LibraryDrag, event: DragEvent, name = '') {
    dragging.value = drag
    const data = event.dataTransfer
    if (!data) return
    const text = drag.kind === 'items' ? `${drag.refs.length}件を移動` : name
    data.setData('text/plain', text)
    data.effectAllowed = 'move'
    if (drag.kind !== 'items' || drag.refs.length < 2) return
    // The browser takes its picture when the handler returns, so the element only needs to last until then. Safari can
    // cancel a drag whose image is wholly off screen, so it sits where the picture will be drawn, below the pointer.
    const image = document.createElement('div')
    image.className = 'library-drag-image'
    image.textContent = text
    image.style.left = `${event.clientX + 12}px`
    image.style.top = `${event.clientY + 12}px`
    document.body.append(image)
    data.setDragImage(image, -12, -12)
    setTimeout(() => image.remove())
  }
  const end = () => { dragging.value = null }
  return { dragging, start, end }
}

// Rows and folders are dragged only on a wide screen with a fine pointer; narrow screens and touch move through the
// dialogs. Decided after mounting, so the server's markup and the first client render agree, and again on every change.
export function useCanDrag(): Readonly<Ref<boolean>> {
  const allowed = ref(false)
  let media: MediaQueryList | undefined
  const update = () => { allowed.value = !!media && !media.matches }
  onMounted(() => {
    media = window.matchMedia('(max-width: 39.999rem), (pointer: coarse)')
    update()
    media.addEventListener('change', update)
  })
  onBeforeUnmount(() => media?.removeEventListener('change', update))
  return allowed
}
