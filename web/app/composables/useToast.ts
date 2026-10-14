export function useToast() {
  const toast = useState<{ message: string } | null>('toast', () => null)
  const showToast = (message: string) => { toast.value = { message } }
  const dismissToast = () => { toast.value = null }
  return { toast, showToast, dismissToast }
}
