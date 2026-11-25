import { z } from 'zod'
import { folderSchema } from '~~/shared/types/folders'
import { privateAPI } from '../../../utils/private-api'
import { limitedJSON, privateHeaders, requireSameOrigin } from '../../../utils/private-request'
// A rename sends the parent too, so that a body without it cannot move the folder to the top.
const bodySchema = z.object({ name: z.string(), parentId: z.string().uuid().nullable() }).strict()
export default defineEventHandler(async event => {
  privateHeaders(event)
  const id = getRouterParam(event, 'id') ?? ''
  if (!z.string().uuid().safeParse(id).success) throw createError({ statusCode: 404 })
  if (event.method !== 'PUT' && event.method !== 'DELETE') throw createError({ statusCode: 405 })
  requireSameOrigin(event)
  if (event.method === 'DELETE') {
    await privateAPI(event, `/my/folders/${id}`, { method: 'DELETE' })
    return sendNoContent(event)
  }
  const body = bodySchema.safeParse(await limitedJSON(event, 16 << 10))
  if (!body.success) throw createError({ statusCode: 400 })
  const result = folderSchema.safeParse(await privateAPI(event, `/my/folders/${id}`, { method: 'PUT', body: body.data }))
  if (!result.success) throw createError({ statusCode: 502 })
  return result.data
})
