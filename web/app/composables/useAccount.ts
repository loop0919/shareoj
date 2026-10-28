import { profileResultSchema, type Profile } from '~~/shared/types/profile'
export function useAccount() {
  const user = useState<{ id: string } | null>('account-user', () => null)
  const profile = useState<Profile | null>('account-profile', () => null)
  async function refreshProfile() {
    const owner = user.value?.id
    const result = profileResultSchema.parse(await $fetch('/api/my/profile'))
    if (user.value?.id !== owner) throw new Error('Account changed')
    profile.value = result.profile
    return result.profile
  }
  async function refreshAccount() {
    const result = await $fetch<{ user: { id: string } | null }>('/api/auth/me')
    if (user.value?.id !== result.user?.id) profile.value = null
    user.value = result.user
    return result.user
  }
  // After the server has ended the session, Cognito clears its own sign-in cookie
  // and sends the browser on to the page; without Google login there is none to clear.
  async function signedOut(result: { logoutUrl?: string } | undefined, page: '/login' | '/') {
    user.value = null
    profile.value = null
    if (result?.logoutUrl) return navigateTo(result.logoutUrl, { external: true })
    await navigateTo(page)
  }
  async function logout() {
    await signedOut(await $fetch<{ logoutUrl?: string } | undefined>('/api/auth/logout', { method: 'POST' }), '/login')
  }
  return { user, profile, refreshAccount, refreshProfile, logout, signedOut }
}
