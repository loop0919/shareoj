import { privateAPI } from '../../../utils/private-api'
import { hasSession } from '../../../utils/private-session'
import { privateHeaders } from '../../../utils/private-request'
export default defineEventHandler(event => {
  privateHeaders(event)
  if (!hasSession(event)) throw createError({ statusCode: 401 })
  const offset = getQuery(event).offset
  return privateAPI(event, `/my/contests/drafts?offset=${encodeURIComponent(typeof offset === 'string' ? offset : '0')}`)
})
