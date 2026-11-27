import { z } from 'zod'
import { iconBatchSize, userHandle } from '~~/shared/types/profile'
import { publicContent } from '../../utils/public-content'
import { upstream } from '../../utils/upstream'

// The icons of a page of names in one request (GET /users?handles=a,b).
export default defineEventHandler(async event => {
  const query = getQuery(event).handles
  const handles = typeof query === 'string' ? query.split(',') : []
  if (!handles.length || handles.length > iconBatchSize || !handles.every(handle => userHandle.test(handle))) throw createError({ statusCode: 400 })
  return upstream(z.object({ items: z.array(z.object({ handle: z.string(), avatar: z.string() })) }), await publicContent(event, `/users?handles=${handles.join(',')}`))
})
