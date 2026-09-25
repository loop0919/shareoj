import { featuredPageSchema } from '~~/shared/types/featured'
import { publicContent } from '../utils/public-content'

export default defineEventHandler(async event => {
  const offset = getQuery(event).offset ?? '0'
  if (typeof offset !== 'string' || !/^\d{1,7}$/.test(offset) || Number(offset) > 1000000) throw createError({ statusCode: 400 })
  const result = featuredPageSchema.safeParse(await publicContent(event, `/featured?offset=${offset}`))
  if (!result.success) throw createError({ statusCode: 502 })
  return result.data
})
