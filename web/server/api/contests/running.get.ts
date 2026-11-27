import { contestScheduleSchema } from '~~/shared/types/contest'
import { publicContent } from '../../utils/public-content'
import { upstream } from '../../utils/upstream'

export default defineEventHandler(async event => {
  return upstream(contestScheduleSchema, await publicContent(event, '/contests/running'))
})
