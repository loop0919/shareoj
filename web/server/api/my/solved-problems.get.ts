import { z } from 'zod'
import { privateAPI } from '../../utils/private-api'
import { privateHeaders } from '../../utils/private-request'

const resultSchema = z.object({ items: z.array(z.string().uuid()) })
export default defineEventHandler(async event => {
  privateHeaders(event)
  const result = resultSchema.safeParse(await privateAPI(event, '/my/solved-problems', { streaming: true }))
  if (!result.success) throw createError({ statusCode: 502 })
  return result.data
})
