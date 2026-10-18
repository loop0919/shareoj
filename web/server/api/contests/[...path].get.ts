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
  if (query.mine === '1' && !hasSession(event)) throw createError({ statusCode: 401 })
  const search = new URLSearchParams({ offset: typeof query.offset === 'string' ? query.offset : '0', mine: typeof query.mine === 'string' ? query.mine : '0' })
  // Staff may ask for the live table while the standings are frozen; the private route answers others with the frozen one.
  if (hasSession(event) && path.endsWith('/standings')) {
    try { return await privateAPI(event, `/my/contests/${path}${query.live === '1' ? '?live=1' : ''}`) }
    catch (error) { if (![401, 403].includes((error as { statusCode?: number }).statusCode ?? 0)) throw error }
  }
  if (hasSession(event) && (!path.includes('/') || path.includes('/problems/') || path.includes('/submissions'))) {
    try { return await privateAPI(event, `/my/contests/${path}?${search}`) }
    catch (error) { if (![401, 403].includes((error as { statusCode?: number }).statusCode ?? 0)) throw error }
  }
  return publicContent(event, `/contests/${path}?${search}`)
})
