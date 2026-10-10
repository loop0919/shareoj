import { createHmac } from 'node:crypto'
import type { H3Event } from 'h3'

// lambda/index.mjs sets the viewer IP. Sign it with the Cognito client secret the
// API already holds, so the API can rate-limit per viewer behind this proxy.
export function clientIPHeaders(event: H3Event): Record<string, string> {
  const ip = getHeader(event, 'x-shareoj-client-ip')
  const secret = useRuntimeConfig(event).cognitoClientSecret
  if (!ip || !secret) return {}
  return { 'X-ShareOJ-Client-IP': ip, 'X-ShareOJ-Client-IP-Signature': createHmac('sha256', secret).update(`shareoj-client-ip:${ip}`).digest('base64') }
}
