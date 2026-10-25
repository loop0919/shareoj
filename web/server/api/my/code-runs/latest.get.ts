import { privateAPI } from '../../../utils/private-api'
import { hasSession } from '../../../utils/private-session'
import { privateHeaders } from '../../../utils/private-request'
import type { CodeRun } from '../../../../shared/types/code-run'

export default defineEventHandler(async event => {
  privateHeaders(event)
  if (!hasSession(event)) throw createError({ statusCode: 401 })
  return privateAPI<CodeRun>(event, '/my/code-runs/latest')
})
