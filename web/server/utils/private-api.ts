import type { H3Event } from 'h3'
import { sessionCookie, refreshCookie, refreshSession } from './private-session'

export async function privateAPI<T>(event: H3Event, path: string, options: { method?: 'GET' | 'POST' | 'PUT' | 'DELETE', body?: unknown, token?: string, timeout?: number, headers?: Record<string, string> } = {}): Promise<T> {
  let token = options.token ?? getCookie(event, sessionCookie)
  const canRefresh = options.token === undefined && (path === '/auth/me' || path.startsWith('/my/'))
  let refreshed = false
  if (!token && canRefresh && getCookie(event, refreshCookie)) { token = await refreshSession(event); refreshed = true }
  const request = () => $fetch<T>(path, {
      baseURL: useRuntimeConfig(event).apiBaseUrl,
      method: options.method ?? 'GET',
      body: options.body as Record<string, unknown> | undefined,
      headers: token ? { ...options.headers, Authorization: `Bearer ${token}` } : options.headers ?? {},
      timeout: options.timeout ?? 12000, retry: 0,
    }) as Promise<T>
  try {
    return await request()
  } catch (initialError) {
    let error = initialError
    if ((error as { response?: { status?: number } }).response?.status === 401 && canRefresh && !refreshed && getCookie(event, refreshCookie)) {
      token = await refreshSession(event)
      try { return await request() } catch (retryError) { error = retryError }
    }
    const status = (error as { response?: { status?: number } }).response?.status
    const upstreamCode = (error as { data?: { error?: string } }).data?.error
    if (status === 429 && ['submission_rate_limited', 'creation_quota_exceeded'].includes(upstreamCode ?? '')) {
      const seconds = Number((error as { response?: { headers?: Headers } }).response?.headers?.get('Retry-After'))
      const retryAfter = Number.isSafeInteger(seconds) && seconds > 0 ? seconds : undefined
      if (retryAfter) setResponseHeader(event, 'Retry-After', retryAfter)
      throw createError({ statusCode: 429, statusMessage: 'Request failed', data: { code: upstreamCode, retryAfter } })
    }
    const code = ['invalid_image', 'image_in_use', 'handle_taken', 'profile_conflict', 'profile_required', 'invalid_avatar', 'invalid_profile', 'tests_not_ready', 'judging_unavailable', 'invalid_submission', 'contest_participation_required', 'contest_participation_unavailable', 'contest_conflict', 'contest_not_ready', 'contest_problem_locked', 'invalid_contest', 'featured_ineligible', 'featured_limit', 'incorrect_password', 'invalid_password', 'password_unavailable', 'confirmation_mismatch', 'account_has_active_contest'].includes(upstreamCode ?? '') ? upstreamCode : undefined
    throw createError({ statusCode: status && [400, 401, 403, 404, 409, 413, 415, 429, 503].includes(status) ? status : 502, statusMessage: 'Request failed', data: code ? { code } : undefined })
  }
}
