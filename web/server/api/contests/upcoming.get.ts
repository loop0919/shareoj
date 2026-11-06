import { contestScheduleSchema } from '~~/shared/types/contest'
import { publicContent } from '../../utils/public-content'

export default defineEventHandler(async event => {
  const result = contestScheduleSchema.safeParse(await publicContent(event, '/contests/upcoming'))
  if (!result.success) throw createError({ statusCode: 502 })
  return result.data
})
