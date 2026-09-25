import { z } from 'zod'
import { featuredStandingsSchema } from '~~/shared/types/featured'
import { publicContent } from '../../utils/public-content'

export default defineEventHandler(async event => {
  const query = z.object({
    at: z.string().datetime({ offset: true }),
    offset: z.string().regex(/^\d{1,7}$/).refine(value => Number(value) <= 1000000).default('0'),
  }).safeParse(getQuery(event))
  if (!query.success) throw createError({ statusCode: 400 })
  const result = featuredStandingsSchema.safeParse(await publicContent(event, `/featured/standings?${new URLSearchParams(query.data)}`))
  if (!result.success) throw createError({ statusCode: 502 })
  return result.data
})
