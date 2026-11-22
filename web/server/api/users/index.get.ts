import { z } from 'zod'
import { iconBatchSize, userHandle } from '~~/shared/types/profile'
import { publicContent } from '../../utils/public-content'

// The icons of a page of names in one request (GET /users?handles=a,b).
export default defineEventHandler(async event => {
  const query = getQuery(event).handles
  const handles = typeof query === 'string' ? query.split(',') : []
  if (!handles.length || handles.length > iconBatchSize || !handles.every(handle => userHandle.test(handle))) throw createError({ statusCode: 400 })
  const result = z.object({ items: z.array(z.object({ handle: z.string(), avatar: z.string() })) })
    .safeParse(await publicContent(event, `/users?handles=${handles.join(',')}`))
  if (!result.success) throw createError({ statusCode: 502 })
  return result.data
})
