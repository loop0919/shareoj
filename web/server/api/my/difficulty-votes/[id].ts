import { z } from 'zod'
import { privateAPI } from '../../../utils/private-api'
import { limitedJSON, privateHeaders, requireSameOrigin } from '../../../utils/private-request'
const difficulty = z.number().int().min(1).max(10)
const resultSchema = z.object({ difficulty: difficulty.nullable(), difficultyAverage: z.number().min(1).max(10).nullable(), difficultyVoteCount: z.number().int().nonnegative() })
export default defineEventHandler(async event => {
  privateHeaders(event)
  const id = getRouterParam(event, 'id') ?? ''
  if (!z.string().uuid().safeParse(id).success) throw createError({ statusCode: 404 })
  const method = event.method
  if (method !== 'GET' && method !== 'PUT' && method !== 'DELETE') throw createError({ statusCode: 405 })
  if (method !== 'GET') requireSameOrigin(event)
  let body: { difficulty: number } | undefined
  if (method === 'PUT') {
    const parsed = z.object({ difficulty }).strict().safeParse(await limitedJSON(event, 1024))
    if (!parsed.success) throw createError({ statusCode: 400 })
    body = parsed.data
  }
  const result = resultSchema.safeParse(await privateAPI(event, `/my/difficulty-votes/${id}`, { method, body }))
  if (!result.success) throw createError({ statusCode: 502 })
  return result.data
})
