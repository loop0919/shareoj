import { featuredPlanSchema } from '~~/shared/types/featured'
import { publicContent } from '../../utils/public-content'

export default defineEventHandler(async event => {
  const result = featuredPlanSchema.safeParse(await publicContent(event, '/featured/schedule'))
  if (!result.success) throw createError({ statusCode: 502 })
  return result.data
})
