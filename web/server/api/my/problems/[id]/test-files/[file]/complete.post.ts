import { privateAPI } from '../../../../../../utils/private-api'
import { hasSession } from '../../../../../../utils/private-session'
import { privateHeaders, requireSameOrigin } from '../../../../../../utils/private-request'
import { uuidPattern } from '../../../../../../utils/route-param'

export default defineEventHandler(async event => {
  privateHeaders(event)
  if (!hasSession(event)) throw createError({ statusCode: 401 })
  requireSameOrigin(event)
  const id = getRouterParam(event, 'id') ?? ''
  const file = getRouterParam(event, 'file') ?? ''
  if (!uuidPattern.test(id) || !uuidPattern.test(file)) throw createError({ statusCode: 404 })
  return privateAPI(event, `/my/problems/${id}/test-files/${file}/complete`, { method: 'POST', timeout: 22000 })
})
