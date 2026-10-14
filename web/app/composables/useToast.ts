type ToastType = 'info' | 'warn' | 'error'

export function useToast() {
  const toast = useState<{ message: string, type: ToastType } | null>('toast', () => null)
  const showToast = (message: string, type: ToastType = 'info') => { toast.value = { message, type } }
  const dismissToast = () => { toast.value = null }
  return { toast, showToast, dismissToast }
}
