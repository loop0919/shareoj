import { privateAPI } from './private-api'
import { hasSession, sessionRefused } from './private-session'
import { privateHeaders } from './private-request'
import { publicContent } from './public-content'
import { streamingMode } from './streaming-mode'
import { uuidPattern } from './route-param'

export default defineEventHandler(async event => {
  privateHeaders(event)
  const id = getRouterParam(event, 'id') ?? ''
  const submission = getRouterParam(event, 'submission')
  if (!uuidPattern.test(id) || (submission && !uuidPattern.test(submission))) throw createError({ statusCode: 404 })
  const query = getQuery(event)
  if (query.mine === '1' && !hasSession(event)) throw createError({ statusCode: 401 })
  const search = new URLSearchParams({ offset: typeof query.offset === 'string' ? query.offset : '0', mine: typeof query.mine === 'string' ? query.mine : '0' })
  const path = `/problems/${id}/submissions${submission ? `/${submission}` : ''}?${search}`
  // Streaming mode keeps only the reader's own list private; the whole list and a detail are public.
  const streaming = streamingMode(event)
  if (hasSession(event) && (!streaming || (query.mine === '1' && !submission))) {
    try { return await privateAPI(event, `/my${path}`, streaming ? { streaming: true } : {}) }
    catch (error) { if (!sessionRefused(error) || query.mine === '1') throw error }
  }
  return publicContent(event, path)
})
