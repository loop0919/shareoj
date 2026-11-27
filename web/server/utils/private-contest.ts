import type { H3Event } from 'h3'
import { privateAPI } from './private-api'
import { hasSession } from './private-session'
import { limitedJSON, privateHeaders, requireSameOrigin } from './private-request'
import { uuidPattern } from './route-param'

// Forwards a request about one contest to /my/contests/{id}/{suffix}. Writes need the same origin, a body is read only
// when limit is set, and DELETE answers 204. A suffix that reads route parameters is a function, called after the checks.
export async function contestAction(event: H3Event, suffix: string | (() => string), limit = 0) {
  privateHeaders(event)
  if (!hasSession(event)) throw createError({ statusCode: 401 })
  const method = event.method as 'GET' | 'POST' | 'PUT' | 'DELETE'
  if (method !== 'GET') requireSameOrigin(event)
  const id = getRouterParam(event, 'id') ?? ''
  if (!uuidPattern.test(id)) throw createError({ statusCode: 404 })
  const path = typeof suffix === 'string' ? suffix : suffix()
  const body = limit ? await limitedJSON(event, limit) : undefined
  const result = await privateAPI(event, `/my/contests/${id}/${path}`, { method, body })
  if (method === 'DELETE') return sendNoContent(event)
  return result
}
