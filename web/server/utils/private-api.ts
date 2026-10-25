import type { H3Event } from 'h3'
import { sessionCookie, refreshCookie, refreshSession } from './private-session'

// Besides retryAfterCodes on a 429, only these upstream codes reach the screen, so upstream diagnostics stay private.
// A code the screen has its own message for goes here, or in retryAfterCodes if it comes with Retry-After.
const forwardedCodes = new Set([
  'incomplete_problem',
  'invalid_schedule',
  'invalid_image',
  'image_in_use',
  'handle_taken',
  'profile_conflict',
  'profile_required',
  'invalid_avatar',
  'invalid_profile',
  'tests_not_ready',
  'judging_unavailable',
  'judge_maintenance',
  'invalid_code_run',
  'code_run_not_found',
  'invalid_submission',
  'contest_participation_required',
  'contest_participation_unavailable',
  'contest_conflict',
  'invalid_clarification',
  'clarification_unavailable',
  'clarification_conflict',
  'contest_not_ready',
  'contest_problem_locked',
  'problem_scoring_locked',
  'contest_participant_locked',
  'contest_participant_problem',
  'contest_invitation_closed',
  'contest_results_pending',
  'setter_not_found',
  'contest_team_unavailable',
  'contest_individual_locked',
  'contest_onsite_unavailable',
  'onsite_irreversible',
  'contest_team_mode_locked',
  'contest_team_size_locked',
  'incorrect_team_code',
  'incorrect_venue_passphrase',
  'invalid_team_name',
  'invalid_venue_passphrase',
  'team_name_taken',
  'team_already_joined',
  'team_full',
  'team_locked',
  'team_not_found',
  'team_member_not_found',
  'participant_not_found',
  'invalid_contest',
  'featured_ineligible',
  'featured_limit',
  'incorrect_password',
  'invalid_password',
  'password_unavailable',
  'confirmation_mismatch',
  'account_has_active_contest',
  'editorial_problem_unavailable',
])

// Passed on with Retry-After only on a 429; they are not in forwardedCodes, so other statuses drop them.
const retryAfterCodes = new Set([
  'submission_rate_limited',
  'code_run_rate_limited',
  'creation_quota_exceeded',
  'contest_attempts_limited',
])

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
    if (status === 429 && retryAfterCodes.has(upstreamCode ?? '')) {
      const seconds = Number((error as { response?: { headers?: Headers } }).response?.headers?.get('Retry-After'))
      const retryAfter = Number.isSafeInteger(seconds) && seconds > 0 ? seconds : undefined
      if (retryAfter) setResponseHeader(event, 'Retry-After', retryAfter)
      throw createError({ statusCode: 429, statusMessage: 'Request failed', data: { code: upstreamCode, retryAfter } })
    }
    const code = forwardedCodes.has(upstreamCode ?? '') ? upstreamCode : undefined
    throw createError({ statusCode: status && [400, 401, 403, 404, 409, 413, 415, 429, 503].includes(status) ? status : 502, statusMessage: 'Request failed', data: code ? { code } : undefined })
  }
}
