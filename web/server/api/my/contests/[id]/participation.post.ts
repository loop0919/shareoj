import { privateAPI } from '../../../../utils/private-api'
import { hasSession } from '../../../../utils/private-session'
import { optionalJSON, privateHeaders, requireSameOrigin } from '../../../../utils/private-request'
// The body creates or joins a team and carries the venue passphrase (ADR 0024); an empty one joins as before.
export default defineEventHandler(async event => {
  privateHeaders(event)
  if (!hasSession(event)) throw createError({ statusCode: 401 })
  requireSameOrigin(event)
  const id = getRouterParam(event, 'id') ?? ''
  if (!/^[a-f0-9-]{36}$/.test(id)) throw createError({ statusCode: 404 })
  const body = await optionalJSON(event, 4 << 10)
  return privateAPI(event, `/my/contests/${id}/participation`, { method: 'POST', ...body ? { body } : {} })
})
