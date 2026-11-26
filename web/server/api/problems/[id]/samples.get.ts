import { problemSamplesSchema } from '~~/shared/types/problem-samples'
import { publicContent } from '../../../utils/public-content'
import { uuidPattern } from '../../../utils/route-param'
import { upstream } from '../../../utils/upstream'

export default defineEventHandler(async event => {
  const id = getRouterParam(event, 'id') ?? ''
  if (!uuidPattern.test(id)) throw createError({ statusCode: 404 })
  return upstream(problemSamplesSchema, await publicContent(event, `/problems/${id}/samples`))
})
