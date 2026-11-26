// A row's actions wait behind its button, in the folder list and in the library table: one menu is open at a time,
// named by its row's key. It closes on Esc, on a press outside `wrap` (the selector of the element holding a row's
// button and menu), or when the focus leaves that element. The caller's template wires the button and the menu.
export function useRowMenu(wrap: string) {
  const menuId = useId()
  const menu = ref('')
  const menuButtons = new Map<string, HTMLElement>()
  // refocus returns the focus to the row's button, for Esc and for actions that start from the menu.
  function closeMenu(refocus = false) {
    const key = menu.value
    menu.value = ''
    if (refocus) menuButtons.get(key)?.focus()
  }
  function leaveMenu(event: FocusEvent) {
    const actions = event.currentTarget as HTMLElement
    if (!(event.relatedTarget instanceof Node && actions.contains(event.relatedTarget))) closeMenu()
  }
  function outside(event: PointerEvent) {
    if (menu.value && event.target instanceof Element && !event.target.closest(wrap)) closeMenu()
  }
  onMounted(() => document.addEventListener('pointerdown', outside))
  onBeforeUnmount(() => document.removeEventListener('pointerdown', outside))
  return { menuId, menu, menuButtons, closeMenu, leaveMenu }
}
