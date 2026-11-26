import { contestScheduleSchema } from '~~/shared/types/contest'
import { privateAPI } from '../../utils/private-api'
import { privateHeaders } from '../../utils/private-request'
import { upstream } from '../../utils/upstream'

export default defineEventHandler(async event => {
  privateHeaders(event)
  return upstream(contestScheduleSchema, await privateAPI(event, '/my/contest-schedule'))
})
