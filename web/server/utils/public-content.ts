import type { H3Event } from 'h3'
import { refusedUnderLoad, retryRefused } from './busy-retry'
// Reads public content from the API. A read the gateway refused under load is sent once more; if it is refused again
// the answer is 503, which tells "busy, try again" apart from 502, an API that failed.
export async function publicContent(event: H3Event, path: string) {
  setResponseHeader(event, 'Cache-Control', 'no-store')
  try { return await retryRefused(() => $fetch(path, { baseURL: useRuntimeConfig(event).apiBaseUrl, timeout: 12000, retry: 0 })) }
  catch (error) {
    const status = (error as { response?: { status?: number } }).response?.status
    if (refusedUnderLoad(error)) throw createError({ statusCode: 503, statusMessage: 'Content service busy' })
    throw createError({ statusCode: status === 404 ? 404 : status === 400 ? 400 : 502, statusMessage: status === 404 ? 'Not found' : 'Content service unavailable' })
  }
}
export function publicCursor(event: H3Event, keys: string[] = ['cursor', 'author', 'tag']) {
  const query = getQuery(event)
  const params = new URLSearchParams()
  for (const key of keys) {
    if (typeof query[key] === 'string') params.set(key, query[key])
  }
  return params.size ? `?${params}` : ''
}
