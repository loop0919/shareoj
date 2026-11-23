import type { H3Event } from 'h3'
import { streamingCode, streamingCookie } from '../../shared/utils/streaming-mode'

export function streamingMode(event: H3Event): boolean {
  return getCookie(event, streamingCookie) === '1'
}

export function refuseStreaming(): never {
  throw createError({ statusCode: 403, statusMessage: 'Streaming mode', data: { code: streamingCode } })
}
