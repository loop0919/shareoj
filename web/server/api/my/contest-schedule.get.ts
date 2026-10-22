import { contestScheduleSchema } from '~~/shared/types/contest'
import { privateAPI } from '../../utils/private-api'
import { privateHeaders } from '../../utils/private-request'

export default defineEventHandler(async event => {
  privateHeaders(event)
  const result = contestScheduleSchema.safeParse(await privateAPI(event, '/my/contest-schedule'))
  if (!result.success) throw createError({ statusCode: 502 })
  return result.data
})
