import { problemSamplesSchema } from '~~/shared/types/problem-samples'
import { publicContent } from '../../../utils/public-content'
import { uuidPattern } from '../../../utils/route-param'

export default defineEventHandler(async event => {
  const id = getRouterParam(event, 'id') ?? ''
  if (!uuidPattern.test(id)) throw createError({ statusCode: 404 })
  const result = problemSamplesSchema.safeParse(await publicContent(event, `/problems/${id}/samples`))
  if (!result.success) throw createError({ statusCode: 502 })
  return result.data
})
