import { notificationsSchema } from '~~/shared/types/notification'
import { privateAPI } from '../../../utils/private-api'
import { privateHeaders, requireSameOrigin } from '../../../utils/private-request'
import { upstream } from '../../../utils/upstream'
export default defineEventHandler(async event => {
  privateHeaders(event)
  requireSameOrigin(event)
  return upstream(notificationsSchema, await privateAPI(event, '/my/notifications/read', { method: 'POST' }))
})
