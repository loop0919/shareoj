import { publicPostSchema } from '~~/shared/types/post'
import { publicContent } from '../../utils/public-content'
import { uuidPattern } from '../../utils/route-param'
export default defineEventHandler(async event => {
  const id = getRouterParam(event, 'id') ?? ''
  if (!uuidPattern.test(id)) throw createError({ statusCode: 404 })
  const result = publicPostSchema.safeParse(await publicContent(event, `/posts/${id}`))
  if (!result.success || result.data.id !== id) throw createError({ statusCode: 502 })
  return result.data
})
