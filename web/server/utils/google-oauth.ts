import type { H3Event } from 'h3'
import { z } from 'zod'
import { sessionTokensSchema } from './private-session'
import { privateAPI } from './private-api'

export const oauthCookie = 'openoj_google_flow'

export function googleOAuthConfig(event: H3Event) {
  const config = useRuntimeConfig(event)
  if (!config.cognitoDomain || !config.cognitoClientId) return null
  try {
    const domain = new URL(config.cognitoDomain)
    if (domain.protocol !== 'https:' || domain.username || domain.password || domain.search || domain.hash || domain.pathname !== '/') return null
    return {
      domain: domain.origin,
      clientId: config.cognitoClientId,
      clientSecret: config.cognitoClientSecret,
      redirectUri: new URL('/auth/google/callback', config.public.siteUrl).href,
    }
  } catch { return null }
}

const tokenSchema = sessionTokensSchema.extend({ token_type: z.string().regex(/^Bearer$/i) })

export async function exchangeGoogleCode(event: H3Event, code: string, verifier: string) {
  const config = googleOAuthConfig(event)
  if (!config) throw createError({ statusCode: 503 })
  const body = new URLSearchParams({ grant_type: 'authorization_code', client_id: config.clientId, code, redirect_uri: config.redirectUri, code_verifier: verifier })
  const headers: Record<string, string> = { 'Content-Type': 'application/x-www-form-urlencoded' }
  if (config.clientSecret) headers.Authorization = `Basic ${Buffer.from(`${config.clientId}:${config.clientSecret}`).toString('base64')}`
  const result = tokenSchema.parse(await $fetch(new URL('/oauth2/token', config.domain).href, { method: 'POST', body: body.toString(), headers, timeout: 12000, retry: 0, redirect: 'error' }))
  await privateAPI<{ id: string }>(event, '/auth/me', { token: result.access_token })
  return result
}
