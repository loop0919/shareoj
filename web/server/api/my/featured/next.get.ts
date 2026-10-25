import { featuredPreviewProblemsSchema } from '~~/shared/types/featured'
import { privateAPI } from '../../../utils/private-api'
import { privateHeaders } from '../../../utils/private-request'

export default defineEventHandler(async event => {
  privateHeaders(event)
  const result = featuredPreviewProblemsSchema.safeParse(await privateAPI(event, '/my/featured/next'))
  if (!result.success) throw createError({ statusCode: 502 })
  return result.data
})
