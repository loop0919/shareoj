import { loginDestination } from '~~/shared/utils/login-destination'
import { createHash, randomBytes } from 'node:crypto'
import { googleOAuthConfig, oauthCookie } from '../../../utils/google-oauth'
import { cookieOptions } from '../../../utils/private-session'
import { privateHeaders } from '../../../utils/private-request'
import { cliFlowSchema } from '../../../utils/cli-oauth'

export default defineEventHandler(event => {
  privateHeaders(event)
  setResponseHeader(event, 'Referrer-Policy', 'no-referrer')
  const config = googleOAuthConfig(event)
  if (!config) return sendRedirect(event, '/login?socialError=unavailable', 303)
  const state = randomBytes(32).toString('base64url')
  const verifier = randomBytes(32).toString('base64url')
  const query = getQuery(event)
  let cli = ''
  let challenge = createHash('sha256').update(verifier).digest('base64url')
  if (['cli_port', 'cli_state', 'cli_challenge'].some(key => query[key] !== undefined)) {
    const flow = cliFlowSchema.safeParse({ port: typeof query.cli_port === 'string' && /^\d{4,5}$/.test(query.cli_port) ? Number(query.cli_port) : 0, state: query.cli_state, challenge: query.cli_challenge })
    if (!flow.success) throw createError({ statusCode: 400, statusMessage: 'Invalid CLI login request' })
    if (!config.clientSecret) throw createError({ statusCode: 503, statusMessage: 'CLI Google login unavailable' })
    cli = `.${Buffer.from(JSON.stringify(flow.data)).toString('base64url')}`
    challenge = flow.data.challenge
  }
  setCookie(event, oauthCookie, `${state}.${verifier}.${Buffer.from(loginDestination(query.next)).toString('base64url')}${cli}`, { ...cookieOptions(event), maxAge: 600 })
  const url = new URL('/oauth2/authorize', config.domain)
  url.search = new URLSearchParams({
    identity_provider: 'Google', response_type: 'code', client_id: config.clientId,
    redirect_uri: config.redirectUri, scope: 'openid email', state,
    code_challenge_method: 'S256', code_challenge: challenge,
  }).toString()
  return sendRedirect(event, url.href, 302)
})
