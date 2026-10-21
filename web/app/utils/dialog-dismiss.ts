import type { ObjectDirective } from 'vue'

export const vDialogDismiss: ObjectDirective<HTMLDialogElement> = {
  mounted(dialog) {
    const outside = (event: MouseEvent) => {
      const bounds = dialog.getBoundingClientRect()
      return event.target === dialog && (event.clientX < bounds.left || event.clientX > bounds.right || event.clientY < bounds.top || event.clientY > bounds.bottom)
    }
    let startedOutside = false
    dialog.addEventListener('pointerdown', event => { startedOutside = event.button === 0 && outside(event) })
    dialog.addEventListener('click', event => {
      const dismiss = startedOutside && outside(event)
      startedOutside = false
      // Use the Escape cancellation path to preserve route guards and busy-state checks.
      if (dismiss && dialog.open && dialog.dispatchEvent(new Event('cancel', { cancelable: true }))) dialog.close()
    })
  },
}
