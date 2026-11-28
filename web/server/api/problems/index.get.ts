import { publicProblemListSchema } from '~~/shared/types/problem'
import { publicContent, publicCursor } from '../../utils/public-content'
import { privateAPI } from '../../utils/private-api'
import { hasSession, sessionRefused } from '../../utils/private-session'
import { privateHeaders } from '../../utils/private-request'
import { upstream } from '../../utils/upstream'

type Solved = '' | 'unsolved' | 'solved'
export default defineEventHandler(async event => {
  // The list pages by number as well as by cursor (ADR 0028).
  const query = publicCursor(event, ['cursor', 'author', 'tag', 'contest', 'sort', 'difficulty', 'offset', 'limit'])
  const asked = getQuery(event).solved
  // An unknown value is not sent on, so the list answers whole.
  if ((asked === 'unsolved' || asked === 'solved') && hasSession(event)) {
    privateHeaders(event)
    try {
      // The filter shows what /api/my/solved-problems shows, so it stays open in streaming mode.
      const list = upstream(publicProblemListSchema, await privateAPI(event, `/my/public-problems${query ? `${query}&` : '?'}solved=${asked}`, { streaming: true }))
      return { ...list, solved: asked as Solved }
    } catch (error) {
      // A session the API refuses falls back to the public list.
      if (!sessionRefused(error)) throw error
    }
  }
  return { ...upstream(publicProblemListSchema, await publicContent(event, `/problems${query}`)), solved: '' as Solved }
})
