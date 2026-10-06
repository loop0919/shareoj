import { cliExchangeSchema, redeemCliTicket } from '../../../utils/cli-oauth'
import { exchangeGoogleCode, googleOAuthConfig } from '../../../utils/google-oauth'
import { limitedJSON, privateHeaders } from '../../../utils/private-request'

export default defineEventHandler(async event => {
  privateHeaders(event)
  const config = googleOAuthConfig(event)
  if (!config?.clientSecret) throw createError({ statusCode: 503, statusMessage: 'CLI Google login unavailable' })
  // This endpoint uses neither browser cookies nor ambient authentication.
  // A signed callback ticket and the CLI-only PKCE verifier are both required.
  const body = cliExchangeSchema.safeParse(await limitedJSON(event, 16 << 10))
  if (!body.success) throw createError({ statusCode: 400, statusMessage: 'Invalid CLI login request' })
  let code: string
  try { code = redeemCliTicket(body.data.ticket, body.data.verifier, config.clientSecret) }
  catch { throw createError({ statusCode: 400, statusMessage: 'Expired or invalid CLI login ticket' }) }
  try {
    const result = await exchangeGoogleCode(event, code, body.data.verifier)
    return { access_token: result.access_token, refresh_token: result.refresh_token, expires_in: result.expires_in }
  } catch {
    throw createError({ statusCode: 401, statusMessage: 'CLI login failed; start a new login' })
  }
})
