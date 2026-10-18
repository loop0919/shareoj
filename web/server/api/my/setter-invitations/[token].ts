import { privateAPI } from '../../../utils/private-api'
import { hasSession } from '../../../utils/private-session'
import { privateHeaders, requireSameOrigin } from '../../../utils/private-request'
// GET previews a co-setter link and POST accepts it, like tester links.
export default defineEventHandler(async event => {
  privateHeaders(event)
  if (!hasSession(event)) throw createError({ statusCode: 401 })
  if (event.method !== 'GET' && event.method !== 'POST') throw createError({ statusCode: 405 })
  if (event.method === 'POST') requireSameOrigin(event)
  const token = getRouterParam(event, 'token') ?? ''
  if (!/^[A-Z2-7]{32}$/.test(token)) throw createError({ statusCode: 404 })
  return privateAPI(event, `/my/setter-invitations/${token}`, { method: event.method })
})
