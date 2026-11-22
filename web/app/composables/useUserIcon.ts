import { deletedHandle, userHandle } from '~~/shared/types/profile'
import { createIconLoader, type UserIcon } from '~/utils/user-icons'

// One loader per browser tab: every name on the page shares a request, and a name seen before is not read again.
// The server never asks, so its copy stays empty and nothing passes between requests.
const loader = createIconLoader(handles => $fetch<{ items: UserIcon[] }>('/api/users', { query: { handles: handles.join(',') }, retry: 0 }).then(result => result.items))

// The icon to show beside a handle: '' until it is read, and for a handle without one.
export function useUserIcon(handle: () => string | undefined) {
  if (import.meta.client) {
    watch(handle, value => { if (value && userHandle.test(value) && !deletedHandle.test(value)) loader.want(value) }, { immediate: true })
  }
  return computed(() => loader.icons.get(handle() ?? '') ?? '')
}
