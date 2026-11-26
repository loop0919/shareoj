import { z } from 'zod'
import { accountsSchema, userHandle } from '~~/shared/types/profile'
import { publicContent } from '../../utils/public-content'
import { upstream } from '../../utils/upstream'

export default defineEventHandler(async event => {
  const handle = getRouterParam(event, 'handle') ?? ''
  if (!userHandle.test(handle)) throw createError({ statusCode: 404 })
  return upstream(z.object({
    accounts: accountsSchema.prefault({}), handle: z.string(), avatar: z.string(), createdAt: z.string().datetime({ offset: true }),
  }), await publicContent(event, `/users/${handle}`))
})
