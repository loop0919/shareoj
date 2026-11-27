import { z } from 'zod'
import { privateAPI } from '../../utils/private-api'
import { limitedJSON, privateHeaders, requireSameOrigin } from '../../utils/private-request'
import { upstream } from '../../utils/upstream'
const viewSchema = z.object({ enabled: z.boolean() })
// The folder view beta is switched apart from the profile, whose save checks its version.
export default defineEventHandler(async event => {
  privateHeaders(event)
  if (event.method !== 'PUT') throw createError({ statusCode: 405 })
  requireSameOrigin(event)
  const body = viewSchema.strict().safeParse(await limitedJSON(event, 16 << 10))
  if (!body.success) throw createError({ statusCode: 400 })
  return upstream(viewSchema, await privateAPI(event, '/my/folder-view', { method: 'PUT', body: body.data }))
})
