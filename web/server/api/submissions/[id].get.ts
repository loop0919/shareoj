import { publicContent } from '../../utils/public-content'
import { privateAPI } from '../../utils/private-api'
import { hasSession } from '../../utils/private-session'
import { privateHeaders } from '../../utils/private-request'
import type { Submission } from '../../../shared/types/submission'

export default defineEventHandler(async event => {
  privateHeaders(event)
  const id = getRouterParam(event, 'id') ?? ''
  if (!/^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$/.test(id)) throw createError({ statusCode: 404 })
  if (hasSession(event)) {
    try { return await privateAPI<Submission>(event, `/my/submissions/${id}/public`) }
    catch (error) { if (![401, 403].includes((error as { statusCode?: number }).statusCode ?? 0)) throw error }
  }
  return publicContent(event, `/submissions/${id}`)
})
