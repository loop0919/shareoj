import { z } from 'zod'
import { publicContent } from '../utils/public-content'
import { cachedRead } from '../utils/cached-read'

const catalog = z.object({ items: z.array(z.object({ id: z.string(), label: z.string() })), maintenance: z.boolean().default(false) })
// Every page reads the catalog, on each render, each move to another page and every 30 seconds, and it changes only
// when the judge is deployed or goes into maintenance. Each server process reads it from the API at most once in 15
// seconds. A read the API refuses under load answers the catalog of the last 5 minutes, because a missing catalog
// makes every open page stop taking submissions.
const cached = cachedRead<z.infer<typeof catalog>>(15_000, 300_000)
export default defineEventHandler(async event => {
  const data = await cached(async () => {
    const result = catalog.safeParse(await publicContent(event, '/runtimes'))
    if (!result.success) throw createError({ statusCode: 502 })
    return result.data
  })
  setResponseHeader(event, 'Cache-Control', 'no-store')
  return data
})
