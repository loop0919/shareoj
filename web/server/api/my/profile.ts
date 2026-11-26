import { profileResultSchema } from '~~/shared/types/profile'
import { privateAPI } from '../../utils/private-api'
import { hasSession } from '../../utils/private-session'
import { privateHeaders, requireSameOrigin, limitedJSON } from '../../utils/private-request'
import { upstream } from '../../utils/upstream'

export default defineEventHandler(async event => {
  privateHeaders(event)
  if (!hasSession(event)) throw createError({ statusCode: 401 })
  if (event.method !== 'GET' && event.method !== 'PUT') throw createError({ statusCode: 405 })
  if (event.method === 'PUT') requireSameOrigin(event)
  const body = event.method === 'PUT' ? await limitedJSON(event, 200 << 10) : undefined
  return upstream(profileResultSchema, await privateAPI(event, '/my/profile', { method: event.method, body, ...(event.method === 'GET' ? { streaming: true as const } : {}) }))
})
