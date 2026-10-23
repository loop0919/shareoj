import { publicProblemListSchema } from '~~/shared/types/problem'
import { publicContent, publicCursor } from '../../utils/public-content'
export default defineEventHandler(async event => {
  // The list pages by number as well as by cursor (ADR 0028).
  const result = publicProblemListSchema.safeParse(await publicContent(event, `/problems${publicCursor(event, ['cursor', 'author', 'tag', 'offset', 'limit'])}`))
  if (!result.success) throw createError({ statusCode: 502 })
  return result.data
})
