import { featuredPlanSchema } from '~~/shared/types/featured'
import { publicContent } from '../../utils/public-content'
import { upstream } from '../../utils/upstream'

export default defineEventHandler(async event => {
  return upstream(featuredPlanSchema, await publicContent(event, '/featured/schedule'))
})
