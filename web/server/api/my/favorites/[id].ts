import { z } from 'zod'
import { privateAPI } from '../../../utils/private-api'
import { limitedJSON, privateHeaders, requireSameOrigin } from '../../../utils/private-request'
const resultSchema = z.object({ favorited: z.boolean(), favoriteCount: z.number().int().nonnegative() })
export default defineEventHandler(async event => {
  privateHeaders(event)
  const id = getRouterParam(event, 'id') ?? ''
  if (!z.string().uuid().safeParse(id).success) throw createError({ statusCode: 404 })
  if (event.method !== 'GET' && event.method !== 'PUT') throw createError({ statusCode: 405 })
  let body: { favorited: boolean } | undefined
  if (event.method === 'PUT') {
    requireSameOrigin(event)
    const parsed = z.object({ favorited: z.boolean() }).strict().safeParse(await limitedJSON(event, 1024))
    if (!parsed.success) throw createError({ statusCode: 400 })
    body = parsed.data
  }
  const result = resultSchema.safeParse(await privateAPI(event, `/my/favorites/${id}`, { method: event.method, body, streaming: true }))
  if (!result.success) throw createError({ statusCode: 502 })
  return result.data
})
