import { notificationsSchema } from '~~/shared/types/notification'
import { privateAPI } from '../../../utils/private-api'
import { privateHeaders } from '../../../utils/private-request'
import { upstream } from '../../../utils/upstream'
export default defineEventHandler(async event => {
  privateHeaders(event)
  const history = getQuery(event).history === '1'
  return upstream(notificationsSchema, await privateAPI(event, history ? '/my/notifications?history=1' : '/my/notifications'))
})
