import { tagListSchema } from '~~/shared/types/tag'
import { publicContent } from '../utils/public-content'
export default defineEventHandler(async event => {
  const result = tagListSchema.safeParse(await publicContent(event, '/tags'))
  if (!result.success) throw createError({ statusCode: 502 })
  return result.data
})
