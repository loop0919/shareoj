import { z } from 'zod'
import { featuredPlanSchema } from '~~/shared/types/featured'
import { privateAPI } from '../../../utils/private-api'
import { privateHeaders } from '../../../utils/private-request'

export default defineEventHandler(async event => {
  privateHeaders(event)
  const query = z.object({
    problem: z.string().uuid().optional(),
    preference: z.enum(['soon', 'later']).optional(),
  }).safeParse(getQuery(event))
  if (!query.success) throw createError({ statusCode: 400 })
  const params = new URLSearchParams(Object.entries(query.data).filter((entry): entry is [string, string] => !!entry[1]))
  const result = featuredPlanSchema.safeParse(await privateAPI(event, `/my/featured/schedule${params.size ? `?${params}` : ''}`))
  if (!result.success) throw createError({ statusCode: 502 })
  return result.data
})
