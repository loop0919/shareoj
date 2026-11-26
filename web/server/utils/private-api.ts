import type { H3Event } from 'h3'
import { sessionCookie, refreshCookie, refreshSession } from './private-session'
import { retryRefused } from './busy-retry'
import { refuseStreaming, streamingMode } from './streaming-mode'
import { forwardedCodes, retryAfterCodes } from '../../shared/utils/error-codes'

// The lists are shared with the app, which types its messages by them.
const forwarded = new Set<string>(forwardedCodes)
const retryAfterOnly = new Set<string>(retryAfterCodes)

export async function privateAPI<T>(event: H3Event, path: string, options: { method?: 'GET' | 'POST' | 'PUT' | 'DELETE', body?: unknown, token?: string, timeout?: number, headers?: Record<string, string>, streaming?: true } = {}): Promise<T> {
  // Streaming mode refuses every call its route has not marked, so a new private route starts closed. A call with its
  // own token is a login in progress, not the browser's session.
  if (options.token === undefined && !options.streaming && streamingMode(event)) refuseStreaming()
  let token = options.token ?? getCookie(event, sessionCookie)
  const canRefresh = options.token === undefined && (path === '/auth/me' || path.startsWith('/my/'))
  let refreshed = false
  if (!token && canRefresh && getCookie(event, refreshCookie)) { token = await refreshSession(event); refreshed = true }
  // A read may be sent again when the gateway refuses it under load. A write is not: a 503 may also be a request that
  // ran past the gateway's time limit.
  const repeatable = (options.method ?? 'GET') === 'GET'
  const send = () => $fetch<T>(path, {
      baseURL: useRuntimeConfig(event).apiBaseUrl,
      method: options.method ?? 'GET',
      body: options.body as Record<string, unknown> | undefined,
      headers: token ? { ...options.headers, Authorization: `Bearer ${token}` } : options.headers ?? {},
      timeout: options.timeout ?? 12000, retry: 0,
    }) as Promise<T>
  const request = () => repeatable ? retryRefused(send) : send()
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
    if (status === 429 && retryAfterOnly.has(upstreamCode ?? '')) {
      const seconds = Number((error as { response?: { headers?: Headers } }).response?.headers?.get('Retry-After'))
      const retryAfter = Number.isSafeInteger(seconds) && seconds > 0 ? seconds : undefined
      if (retryAfter) setResponseHeader(event, 'Retry-After', retryAfter)
      throw createError({ statusCode: 429, statusMessage: 'Request failed', data: { code: upstreamCode, retryAfter } })
    }
    const code = forwarded.has(upstreamCode ?? '') ? upstreamCode : undefined
    throw createError({ statusCode: status && [400, 401, 403, 404, 409, 413, 415, 429, 503].includes(status) ? status : 502, statusMessage: 'Request failed', data: code ? { code } : undefined })
  }
}
