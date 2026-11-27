import { tagListSchema } from '~~/shared/types/tag'
import { publicContent } from '../utils/public-content'
import { upstream } from '../utils/upstream'
export default defineEventHandler(async event => {
  return upstream(tagListSchema, await publicContent(event, '/tags'))
})
