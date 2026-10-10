import { z } from 'zod'
import { googleOAuthConfig } from '../../utils/google-oauth'
import { privateAPI } from '../../utils/private-api'
import { clearPrivateSession, saveSession, sessionTokensSchema, hasSession } from '../../utils/private-session'
import { limitedJSON, privateHeaders, requireSameOrigin } from '../../utils/private-request'

const resultSchema = z.object({
  refresh_token: z.string().min(1).max(3800).optional(), access_token: z.string().optional(), expires_in: z.number().int().positive().optional(),
  challenge_name: z.string().optional(), challenge_parameters: z.record(z.string(), z.string()).optional(), session: z.string().optional(),
})

export default defineEventHandler(async event => {
  privateHeaders(event)
  const action = getRouterParam(event, 'action')
  if (action === 'providers' && event.method === 'GET') {
    const google = googleOAuthConfig(event)
    return { google: Boolean(google), googleCLI: Boolean(google?.clientSecret) }
  }
  if (action === 'me' && event.method === 'GET') {
    if (!hasSession(event)) return { user: null }
    try { return { user: await privateAPI<{ id: string }>(event, '/auth/me') } }
    catch (error) {
      if ((error as { statusCode?: number }).statusCode !== 401) throw error
      clearPrivateSession(event)
      return { user: null }
    }
  }
  if (!['login', 'challenge', 'logout', 'signup', 'confirm-signup', 'resend-confirmation'].includes(action ?? '') || event.method !== 'POST') throw createError({ statusCode: 404 })
  requireSameOrigin(event)
  if (action === 'logout') {
    clearPrivateSession(event)
    return { user: null }
  }
  const body = await limitedJSON(event, 16 << 10)
  if (['signup', 'confirm-signup', 'resend-confirmation'].includes(action ?? '')) {
    const schema = action === 'resend-confirmation' ? z.object({ sent: z.literal(true) }) : z.object({ confirmed: z.boolean() })
    const result = schema.safeParse(await privateAPI(event, `/auth/${action}`, { method: 'POST', body, headers: clientIPHeaders(event) }))
    if (!result.success) throw createError({ statusCode: 502 })
    return result.data
  }
  const result = resultSchema.safeParse(await privateAPI(event, `/auth/${action}`, { method: 'POST', body }))
  if (!result.success) throw createError({ statusCode: 502 })
  const data = result.data
  if (data.challenge_name && data.session) {
    return { challengeName: data.challenge_name, challengeParameters: data.challenge_parameters ?? {}, session: data.session }
  }
  if (!data.access_token || data.access_token.length > 3800 || !data.expires_in) throw createError({ statusCode: 502 })
  const user = await privateAPI<{ id: string }>(event, '/auth/me', { token: data.access_token })
  saveSession(event, sessionTokensSchema.parse(data), true)
  return { user }
})
