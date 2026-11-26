import { z } from 'zod'
import { folderSchema, folderTreeSchema } from '~~/shared/types/folders'
import { privateAPI } from '../../../utils/private-api'
import { limitedJSON, privateHeaders, requireSameOrigin } from '../../../utils/private-request'
import { upstream } from '../../../utils/upstream'
// The API checks the name; a null parent is the top.
const bodySchema = z.object({ name: z.string(), parentId: z.string().uuid().nullable() }).strict()
export default defineEventHandler(async event => {
  privateHeaders(event)
  if (event.method !== 'GET' && event.method !== 'POST') throw createError({ statusCode: 405 })
  if (event.method === 'GET') {
    return upstream(folderTreeSchema, await privateAPI(event, '/my/folders'))
  }
  requireSameOrigin(event)
  const body = bodySchema.safeParse(await limitedJSON(event, 16 << 10))
  if (!body.success) throw createError({ statusCode: 400 })
  const result = folderSchema.safeParse(await privateAPI(event, '/my/folders', { method: 'POST', body: body.data }))
  if (!result.success) throw createError({ statusCode: 502 })
  setResponseStatus(event, 201)
  return result.data
})
