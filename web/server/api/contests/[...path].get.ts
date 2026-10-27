import { publicContent } from '../../utils/public-content'
import { privateAPI } from '../../utils/private-api'
import { hasSession } from '../../utils/private-session'
import { privateHeaders } from '../../utils/private-request'
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
  // Only staff who chose the table without the freeze go to the private route (ADR 0023); it answers others with the
  // frozen table, which everyone else gets from the public route, so the default view never depends on a session.
  if (hasSession(event) && path.endsWith('/standings') && query.live === '1') {
    try { return await privateAPI(event, `/my/contests/${path}?live=1${onsite}`) }
    catch (error) { if (![401, 403].includes((error as { statusCode?: number }).statusCode ?? 0)) throw error }
  }
  if (hasSession(event) && (!path.includes('/') || path.includes('/problems/') || path.includes('/submissions'))) {
    try { return await privateAPI(event, `/my/contests/${path}?${search}`) }
    catch (error) { if (![401, 403].includes((error as { statusCode?: number }).statusCode ?? 0)) throw error }
  }
  return publicContent(event, `/contests/${path}?${search}`)
})
