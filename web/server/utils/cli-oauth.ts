import { createHash, createHmac, timingSafeEqual } from 'node:crypto'
import { z } from 'zod'

const nonce = z.string().regex(/^[A-Za-z0-9_-]{43}$/)
export const cliFlowSchema = z.object({
  port: z.number().int().min(1024).max(65535),
  state: nonce,
  challenge: nonce,
})
const ticketSchema = z.object({ code: z.string().min(1).max(4096), challenge: nonce, expires: z.number().int() })
export const cliExchangeSchema = z.object({ ticket: z.string().min(1).max(8192), verifier: nonce })

function signature(payload: string, secret: string) {
  return createHmac('sha256', secret).update(`shareoj-cli-login-v1:${payload}`).digest('base64url')
}

export function cliTicket(code: string, challenge: string, secret: string, now = Date.now()) {
  const payload = Buffer.from(JSON.stringify({ code, challenge, expires: now + 60000 })).toString('base64url')
  return `${payload}.${signature(payload, secret)}`
}

export function redeemCliTicket(ticket: string, verifier: string, secret: string, now = Date.now()) {
  const [payload = '', signed = '', extra] = ticket.split('.')
  const expected = signature(payload, secret)
  if (extra !== undefined || signed.length !== expected.length || !/^[A-Za-z0-9_-]+$/.test(payload)
    || !timingSafeEqual(Buffer.from(signed), Buffer.from(expected))) throw new Error('Invalid CLI ticket')
  const data = ticketSchema.parse(JSON.parse(Buffer.from(payload, 'base64url').toString('utf8')))
  if (data.expires <= now || data.expires > now + 60000
    || createHash('sha256').update(verifier).digest('base64url') !== data.challenge) throw new Error('Expired or mismatched CLI ticket')
  return data.code
}
