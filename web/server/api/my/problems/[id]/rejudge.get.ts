import { privateAPI } from '../../../../utils/private-api'
import { hasSession } from '../../../../utils/private-session'
import { privateHeaders } from '../../../../utils/private-request'
import { uuidPattern } from '../../../../utils/route-param'

// Answers 404 to viewers who cannot rejudge, so pages also use it to decide whether to offer rejudging.
export default defineEventHandler(async event => {
  privateHeaders(event)
  if (!hasSession(event)) throw createError({ statusCode: 401 })
  const id = getRouterParam(event, 'id') ?? ''
  if (!uuidPattern.test(id)) throw createError({ statusCode: 404 })
  return privateAPI<{ pending: number }>(event, `/my/problems/${id}/rejudge`)
})
