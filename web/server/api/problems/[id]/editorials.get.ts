import { userEditorialListSchema } from '~~/shared/types/post'
import { publicContent } from '../../../utils/public-content'
import { uuidPattern } from '../../../utils/route-param'

export default defineEventHandler(async event => {
  const id = getRouterParam(event, 'id') ?? ''
  if (!uuidPattern.test(id)) throw createError({ statusCode: 404 })
  const result = userEditorialListSchema.safeParse(await publicContent(event, `/problems/${id}/editorials`))
  if (!result.success) throw createError({ statusCode: 502 })
  return result.data
})
