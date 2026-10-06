import { loginDestination } from '~~/shared/utils/login-destination'
import { profileResultSchema } from '~~/shared/types/profile'
import { timingSafeEqual } from 'node:crypto'
import { exchangeGoogleCode, googleOAuthConfig, oauthCookie } from '../../../utils/google-oauth'
import { cliFlowSchema, cliTicket } from '../../../utils/cli-oauth'
import { privateAPI } from '../../../utils/private-api'
import { cookieOptions, saveSession } from '../../../utils/private-session'
import { privateHeaders } from '../../../utils/private-request'

export default defineEventHandler(async event => {
  privateHeaders(event)
  setResponseHeader(event, 'Referrer-Policy', 'no-referrer')
  const flow = getCookie(event, oauthCookie) ?? ''
  deleteCookie(event, oauthCookie, cookieOptions(event))
  const query = getQuery(event)
  const config = googleOAuthConfig(event)
  const [state, verifier, encodedNext = '', encodedCli] = flow.split('.')
  const next = loginDestination(Buffer.from(encodedNext, 'base64url').toString('utf8'))
  if (!config || !state || !verifier || !/^[\w-]{43}$/.test(state) || !/^[\w-]{43}$/.test(verifier)
    || typeof query.state !== 'string' || Buffer.byteLength(query.state) !== Buffer.byteLength(state)
    || !timingSafeEqual(Buffer.from(query.state), Buffer.from(state))) {
    return sendRedirect(event, '/login?socialError=failed', 303)
  }
  try {
    const validCode = !query.error && typeof query.code === 'string' && query.code.length > 0 && query.code.length <= 4096
    if (encodedCli && config.clientSecret) {
      const cli = cliFlowSchema.parse(JSON.parse(Buffer.from(encodedCli, 'base64url').toString('utf8')))
      const callback = new URL(`http://127.0.0.1:${cli.port}/callback`)
      callback.searchParams.set('state', cli.state)
      if (validCode) callback.searchParams.set('ticket', cliTicket(query.code as string, cli.challenge, config.clientSecret))
      else callback.searchParams.set('error', 'login_failed')
      return sendRedirect(event, callback.href, 303)
    }
    if (!validCode) return sendRedirect(event, '/login?socialError=failed', 303)
    const result = await exchangeGoogleCode(event, query.code as string, verifier)
    const account = profileResultSchema.parse(await privateAPI(event, '/my/profile', { token: result.access_token }))
    saveSession(event, result, true)
    return sendRedirect(event, account.profile ? next : (next === '/my' ? '/onboarding' : `/onboarding?${new URLSearchParams({ next })}`), 303)
  } catch {
    return sendRedirect(event, '/login?socialError=failed', 303)
  }
})
