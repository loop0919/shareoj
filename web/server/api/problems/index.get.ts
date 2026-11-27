import { publicProblemListSchema } from '~~/shared/types/problem'
import { publicContent, publicCursor } from '../../utils/public-content'
import { upstream } from '../../utils/upstream'
export default defineEventHandler(async event => {
  // The list pages by number as well as by cursor (ADR 0028).
  return upstream(publicProblemListSchema, await publicContent(event, `/problems${publicCursor(event, ['cursor', 'author', 'tag', 'contest', 'offset', 'limit'])}`))
})
