import { privateAPI } from '../../../../../../utils/private-api'
import { hasSession } from '../../../../../../utils/private-session'
import { privateHeaders, requireSameOrigin } from '../../../../../../utils/private-request'

export default defineEventHandler(async event => {
  privateHeaders(event)
  if (!hasSession(event)) throw createError({ statusCode: 401 })
  requireSameOrigin(event)
  const id = getRouterParam(event, 'id') ?? ''
  const file = getRouterParam(event, 'file') ?? ''
  if (!/^[a-f0-9-]{36}$/.test(id) || !/^[a-f0-9-]{36}$/.test(file)) throw createError({ statusCode: 404 })
  return privateAPI(event, `/my/problems/${id}/test-files/${file}/complete`, { method: 'POST', timeout: 22000 })
})
