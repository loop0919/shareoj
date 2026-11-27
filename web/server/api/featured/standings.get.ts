import { z } from 'zod'
import { featuredStandingsSchema } from '~~/shared/types/featured'
import { publicContent } from '../../utils/public-content'
import { upstream } from '../../utils/upstream'

export default defineEventHandler(async event => {
  const query = z.object({
    at: z.string().datetime({ offset: true }),
    offset: z.string().regex(/^\d{1,7}$/).refine(value => Number(value) <= 1000000).default('0'),
  }).safeParse(getQuery(event))
  if (!query.success) throw createError({ statusCode: 400 })
  return upstream(featuredStandingsSchema, await publicContent(event, `/featured/standings?${new URLSearchParams(query.data)}`))
})
