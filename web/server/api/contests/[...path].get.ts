import { publicContent } from '../../utils/public-content'
import { privateAPI } from '../../utils/private-api'
import { hasSession } from '../../utils/private-session'
import { privateHeaders } from '../../utils/private-request'
import { refuseStreaming, streamingContestView, streamingMode } from '../../utils/streaming-mode'
export default defineEventHandler(async event => {
  privateHeaders(event)
  const path = getRouterParam(event, 'path') ?? ''
  const uuid = '[a-f0-9-]{36}'
  if (!new RegExp(`^${uuid}(?:/(?:standings|editorials|problems/${uuid}(?:/submissions)?|submissions(?:/${uuid})?))?$`).test(path)) throw createError({ statusCode: 404 })
  const query = getQuery(event)
  if ((query.mine === '1' || query.mine === 'team') && !hasSession(event)) throw createError({ statusCode: 401 })
  const search = new URLSearchParams({ offset: typeof query.offset === 'string' ? query.offset : '0', mine: typeof query.mine === 'string' ? query.mine : '0' })
  if (typeof query.limit === 'string') search.set('limit', query.limit)
  // The onsite division filters the standings and the submission lists (ADR 0024); the contest's list also narrows to
  // one problem by its ID (ADR 0034).
  const onsite = query.scope === 'onsite' ? '&scope=onsite' : ''
  if (onsite) search.set('scope', 'onsite')
  if (typeof query.problem === 'string' && new RegExp(`^${uuid}$`).test(query.problem)) search.set('problem', query.problem)
  const mine = query.mine === '1' || query.mine === 'team'
  const sessionRoute = !path.includes('/') || path.includes('/problems/') || path.includes('/submissions')
  // Streaming mode never shows the unfrozen table; the standings and the editorials answer from the public route for
  // every role, without a check. Where the session route would be used, staff are answered like a signed-out visitor,
  // so what only staff can read stays off the screen, and their own submissions are refused; everyone else keeps the
  // session's view. A failed check throws instead of falling back to either.
  const streaming = streamingMode(event)
  const staffView = streaming && (sessionRoute || mine) && await streamingContestView(event, path.split('/')[0] ?? '') === 'public'
  if (staffView && mine) refuseStreaming()
  // Only staff who chose the table without the freeze go to the private route (ADR 0023); it answers others with the
  // frozen table, which everyone else gets from the public route, so the default view never depends on a session.
  if (hasSession(event) && !streaming && path.endsWith('/standings') && query.live === '1') {
    try { return await privateAPI(event, `/my/contests/${path}?live=1${onsite}`) }
    catch (error) { if (![401, 403].includes((error as { statusCode?: number }).statusCode ?? 0)) throw error }
  }
  if (hasSession(event) && !staffView && sessionRoute) {
    try { return await privateAPI(event, `/my/contests/${path}?${search}`, { streaming: true }) }
    catch (error) { if (![401, 403].includes((error as { statusCode?: number }).statusCode ?? 0)) throw error }
  }
  return publicContent(event, `/contests/${path}?${search}`)
})
