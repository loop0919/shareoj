import { privateAPI } from '../../../../utils/private-api'
import { hasSession } from '../../../../utils/private-session'
import { privateHeaders } from '../../../../utils/private-request'

// Answers 404 to viewers who cannot rejudge, so pages also use it to decide whether to offer rejudging.
export default defineEventHandler(async event => {
  privateHeaders(event)
  if (!hasSession(event)) throw createError({ statusCode: 401 })
  const id = getRouterParam(event, 'id') ?? ''
  if (!/^[a-f0-9-]{36}$/.test(id)) throw createError({ statusCode: 404 })
  return privateAPI<{ pending: number }>(event, `/my/problems/${id}/rejudge`)
})
