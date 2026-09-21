import { problemSamplesSchema } from '~~/shared/types/problem-samples'
import { publicContent } from '../../../utils/public-content'

export default defineEventHandler(async event => {
  const id = getRouterParam(event, 'id') ?? ''
  if (!/^[a-f0-9-]{36}$/.test(id)) throw createError({ statusCode: 404 })
  const result = problemSamplesSchema.safeParse(await publicContent(event, `/problems/${id}/samples`))
  if (!result.success) throw createError({ statusCode: 502 })
  return result.data
})
