import { privateAPI } from '../../../utils/private-api'
import { privateHeaders, requireSameOrigin } from '../../../utils/private-request'
import { uuidPattern } from '../../../utils/route-param'

export default defineEventHandler(async (event) => {
  privateHeaders(event)
  requireSameOrigin(event)
  const id = getRouterParam(event, 'id') ?? ''
  if (!uuidPattern.test(id)) throw createError({ statusCode: 404 })
  await privateAPI(event, `/my/images/${id}`, { method: 'DELETE' })
  setResponseStatus(event, 204)
})
