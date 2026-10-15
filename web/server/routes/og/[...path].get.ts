import { z } from 'zod'
import { socialPages } from '~~/shared/social-pages'
import { labelIndex } from '~~/shared/types/contest'
import { fullId } from '~~/shared/utils/short-id'
import { publicContent } from '../../utils/public-content'
import { renderShareImage } from '../../utils/share-image'

export default defineEventHandler(async event => {
  setResponseHeader(event, 'Cache-Control', 'no-store')
  const raw = getRouterParam(event, 'path') ?? ''
  if (!raw.endsWith('.png')) throw createError({ statusCode: 404 })
  const path = raw === 'index.png' ? '/' : `/${raw.slice(0, -4)}`
  let content = Object.hasOwn(socialPages, path) ? socialPages[path] : undefined
  if (!content) {
    const match = /^\/(problems|contests|blog)\/([0-9A-Za-z]{22})(?:\/([A-Z]{1,3}))?$/.exec(path)
    if (!match || (match[3] && match[1] !== 'contests')) throw createError({ statusCode: 404 })
    const id = fullId(match[2])
    let apiPath = `/${match[1] === 'blog' ? 'posts' : match[1]}/${id}`
    let expected = id
    if (match[3]) {
      const contest = z.object({ status: z.enum(['scheduled', 'running', 'ended']), problems: z.array(z.object({ id: z.string() })) }).safeParse(await publicContent(event, `/contests/${id}`))
      if (!contest.success) throw createError({ statusCode: 502 })
      expected = contest.data.problems[labelIndex(match[3])]?.id ?? ''
      if (contest.data.status === 'scheduled' || !expected) throw createError({ statusCode: 404 })
      apiPath = `/contests/${id}/problems/${expected}`
    }
    // Fetch the public API directly: never forward a session or fall back to private content.
    const result = z.object({ id: z.string(), title: z.string().min(1).max(1000) }).safeParse(await publicContent(event, apiPath))
    if (!result.success || result.data.id !== expected) throw createError({ statusCode: 502 })
    content = { title: result.data.title, kind: match[1] === 'blog' ? 'ARTICLE' : match[1] === 'problems' || match[3] ? 'PROBLEM' : 'CONTEST' }
  }
  const image = await renderShareImage(content.title, content.kind)
  setResponseHeader(event, 'Content-Type', 'image/png')
  setResponseHeader(event, 'X-Content-Type-Options', 'nosniff')
  return image
})
