import { privateAPI } from '../../../../../utils/private-api'
import { hasSession } from '../../../../../utils/private-session'
import { privateHeaders, requireSameOrigin } from '../../../../../utils/private-request'
// The owner removes a co-setter, or a co-setter removes their own handle to leave.
export default defineEventHandler(async event => {
  privateHeaders(event)
  if (!hasSession(event)) throw createError({ statusCode: 401 })
  requireSameOrigin(event)
  const id = getRouterParam(event, 'id') ?? ''
  const handle = getRouterParam(event, 'handle') ?? ''
  if (!/^[a-f0-9-]{36}$/.test(id) || !/^[a-z][a-z0-9_]{2,19}$/.test(handle)) throw createError({ statusCode: 404 })
  await privateAPI(event, `/my/contests/${id}/setters/${handle}`, { method: 'DELETE' })
  return sendNoContent(event)
})
