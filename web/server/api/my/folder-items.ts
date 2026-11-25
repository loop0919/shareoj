import { z } from 'zod'
import { folderRefSchema, maxFolderItems } from '~~/shared/types/folders'
import { privateAPI } from '../../utils/private-api'
import { limitedJSON, privateHeaders, requireSameOrigin } from '../../utils/private-request'
// A null folderId makes the items unsorted; the key is still required, so that a body without it changes nothing.
const bodySchema = z.object({ folderId: z.string().uuid().nullable(), items: z.array(folderRefSchema.strict()).min(1).max(maxFolderItems) }).strict()
export default defineEventHandler(async event => {
  privateHeaders(event)
  if (event.method !== 'PUT') throw createError({ statusCode: 405 })
  requireSameOrigin(event)
  const body = bodySchema.safeParse(await limitedJSON(event, 16 << 10))
  if (!body.success) throw createError({ statusCode: 400 })
  await privateAPI(event, '/my/folder-items', { method: 'PUT', body: body.data })
  return sendNoContent(event)
})
