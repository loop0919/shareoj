import { userEditorialListSchema } from '~~/shared/types/post'
import { publicContent } from '../../../utils/public-content'

export default defineEventHandler(async event => {
  const id = getRouterParam(event, 'id') ?? ''
  if (!/^[a-f0-9-]{36}$/.test(id)) throw createError({ statusCode: 404 })
  const result = userEditorialListSchema.safeParse(await publicContent(event, `/problems/${id}/editorials`))
  if (!result.success) throw createError({ statusCode: 502 })
  return result.data
})
