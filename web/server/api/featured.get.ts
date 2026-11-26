import { featuredPageSchema } from '~~/shared/types/featured'
import { publicContent } from '../utils/public-content'
import { upstream } from '../utils/upstream'

export default defineEventHandler(async event => {
  const offset = getQuery(event).offset ?? '0'
  if (typeof offset !== 'string' || !/^\d{1,7}$/.test(offset) || Number(offset) > 1000000) throw createError({ statusCode: 400 })
  return upstream(featuredPageSchema, await publicContent(event, `/featured?offset=${offset}`))
})
