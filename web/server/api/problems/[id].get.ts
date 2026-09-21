import { publicProblemSchema } from '~~/shared/types/problem'
import { publicContent } from '../../utils/public-content'
import { accountProblemSchema } from '~~/shared/types/account-problems'
import { privateAPI } from '../../utils/private-api'
import { hasSession } from '../../utils/private-session'
import { privateHeaders } from '../../utils/private-request'
export default defineEventHandler(async event => {
  privateHeaders(event)
  const id = getRouterParam(event, 'id') ?? ''
  if (!/^[a-f0-9-]{36}$/.test(id)) throw createError({ statusCode: 404 })
  let content: unknown
  try { content = await publicContent(event, `/problems/${id}`) }
  catch (error) {
    if ((error as { statusCode?: number }).statusCode !== 404 || !hasSession(event)) throw error
    let saved: unknown
    try { saved = await privateAPI(event, `/my/problems/${id}`) }
    catch (privateError) {
      if ([401, 403, 404].includes((privateError as { statusCode?: number }).statusCode ?? 0)) throw error
      throw privateError
    }
    const parsed = accountProblemSchema.safeParse(saved)
    if (!parsed.success || parsed.data.id !== id) throw createError({ statusCode: 502 })
    const { draft } = parsed.data
    return {
      id, difficultyDistribution: Array<number>(10).fill(0), difficultyAverage: null, difficultyVoteCount: 0, difficulty: draft.difficulty, favoriteCount: 0, title: draft.title.trim() || '無題の問題', markdown: draft.markdown, editorial: draft.editorial,
      timeLimitMs: Number(draft.timeLimitMs), memoryLimitMb: Number(draft.memoryLimitMb),
      hasSamples: draft.testCases.some(test => test.isSample), author: parsed.data.author, testers: parsed.data.testers, isPrivate: true, specialJudge: draft.checker !== null, interactive: draft.interactor !== null,
    }
  }
  const result = publicProblemSchema.safeParse(content)
  if (!result.success || result.data.id !== id) throw createError({ statusCode: 502 })
  return { ...result.data, isPrivate: false }
})
