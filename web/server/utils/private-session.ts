import type { H3Event } from 'h3'
import { z } from 'zod'

export const sessionCookie = 'openoj_access'
export const refreshCookie = 'openoj_refresh'
export const sessionTokensSchema = z.object({
  access_token: z.string().min(1).max(3800), expires_in: z.number().int().positive(),
  refresh_token: z.string().min(1).max(3800).optional(),
})

export function hasSession(event: H3Event) {
  return Boolean(getCookie(event, sessionCookie) || getCookie(event, refreshCookie))
}

// Whether a call made with the session was refused (401 or 403). A route that also has a public answer falls back to it then.
export function sessionRefused(error: unknown) {
  return [401, 403].includes((error as { statusCode?: number }).statusCode ?? 0)
}

export function clearPrivateSession(event: H3Event) {
  deleteCookie(event, sessionCookie, cookieOptions(event))
  deleteCookie(event, refreshCookie, cookieOptions(event))
}

export function saveSession(event: H3Event, tokens: z.infer<typeof sessionTokensSchema>, login = false) {
  setCookie(event, sessionCookie, tokens.access_token, { ...cookieOptions(event), maxAge: Math.min(tokens.expires_in, 86400) })
  // Matches the Cognito app client's 30-day refresh validity; Cognito enforces expiry.
  if (tokens.refresh_token) setCookie(event, refreshCookie, tokens.refresh_token, { ...cookieOptions(event), maxAge: 30 * 86400 })
  else if (login) deleteCookie(event, refreshCookie, cookieOptions(event))
}

export async function refreshSession(event: H3Event) {
  const refreshToken = getCookie(event, refreshCookie)
  if (!refreshToken) throw createError({ statusCode: 401 })
  try {
    const tokens = sessionTokensSchema.parse(await $fetch('/auth/refresh', {
      baseURL: useRuntimeConfig(event).apiBaseUrl, method: 'POST', body: { refresh_token: refreshToken }, timeout: 12000, retry: 0,
    }))
    saveSession(event, tokens)
    return tokens.access_token
  } catch (error) {
    const status = (error as { response?: { status?: number } }).response?.status
    if (status === 401) clearPrivateSession(event)
    throw createError({ statusCode: status && [401, 429, 503].includes(status) ? status : 502, statusMessage: 'Session refresh failed' })
  }
}

export function cookieOptions(event: H3Event) {
  return { httpOnly: true, sameSite: 'lax' as const, secure: new URL(useRuntimeConfig(event).public.siteUrl).protocol === 'https:', path: '/' }
}

