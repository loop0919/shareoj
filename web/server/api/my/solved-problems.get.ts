import { z } from 'zod'
import { privateAPI } from '../../utils/private-api'
import { privateHeaders } from '../../utils/private-request'
import { upstream } from '../../utils/upstream'

const resultSchema = z.object({ items: z.array(z.string().uuid()) })
export default defineEventHandler(async event => {
  privateHeaders(event)
  return upstream(resultSchema, await privateAPI(event, '/my/solved-problems', { streaming: true }))
})
