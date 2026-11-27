import { featuredApplicationsSchema } from '~~/shared/types/featured'
import { privateAPI } from '../../utils/private-api'
import { privateHeaders } from '../../utils/private-request'
import { upstream } from '../../utils/upstream'

export default defineEventHandler(async event => {
  privateHeaders(event)
  return upstream(featuredApplicationsSchema, await privateAPI(event, '/my/featured'))
})
