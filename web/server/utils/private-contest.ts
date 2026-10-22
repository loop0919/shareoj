import type { H3Event } from 'h3'
import { privateAPI } from './private-api'
import { hasSession } from './private-session'
import { limitedJSON, privateHeaders, requireSameOrigin } from './private-request'

// Forwards a request about one contest to /my/contests/{id}/{suffix}. Writes need the same origin, a body is read only
// when limit is set, and DELETE answers 204. A suffix that reads route parameters is a function, called after the checks.
export async function contestAction(event: H3Event, suffix: string | (() => string), limit = 0) {
  privateHeaders(event)
  if (!hasSession(event)) throw createError({ statusCode: 401 })
  const method = event.method as 'GET' | 'POST' | 'PUT' | 'DELETE'
  if (method !== 'GET') requireSameOrigin(event)
  const id = getRouterParam(event, 'id') ?? ''
  if (!/^[a-f0-9-]{36}$/.test(id)) throw createError({ statusCode: 404 })
  const path = typeof suffix === 'string' ? suffix : suffix()
  const body = limit ? await limitedJSON(event, limit) : undefined
  const result = await privateAPI(event, `/my/contests/${id}/${path}`, { method, body })
  if (method === 'DELETE') return sendNoContent(event)
  return result
}

// A route parameter that must match the pattern, or the route answers 404.
export function contestParam(event: H3Event, name: string, pattern: RegExp) {
  const value = getRouterParam(event, name) ?? ''
  if (!pattern.test(value)) throw createError({ statusCode: 404 })
  return value
}
export const handlePattern = /^[a-z][a-z0-9_]{2,19}$/
export const uuidPattern = /^[a-f0-9-]{36}$/
