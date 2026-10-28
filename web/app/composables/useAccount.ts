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
  async function logout() {
    const result = await $fetch<{ logoutUrl?: string } | undefined>('/api/auth/logout', { method: 'POST' })
    user.value = null
    profile.value = null
    // Cognito clears its own sign-in cookie, then sends the browser back to /login.
    if (result?.logoutUrl) return navigateTo(result.logoutUrl, { external: true })
    await navigateTo('/login')
  }
  return { user, profile, refreshAccount, refreshProfile, logout }
}
