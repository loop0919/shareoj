import { postListSchema } from '~~/shared/types/post'
import { publicContent, publicCursor } from '../../utils/public-content'
import { upstream } from '../../utils/upstream'
export default defineEventHandler(async event => {
  return upstream(postListSchema, await publicContent(event, `/posts${publicCursor(event)}`))
})
