import type { H3Event } from 'h3'
import { streamingCode, streamingCookie } from '../../shared/utils/streaming-mode'
import { privateAPI } from './private-api'
import { hasSession, sessionRefused } from './private-session'

export function streamingMode(event: H3Event): boolean {
  return getCookie(event, streamingCookie) === '1'
}

export function refuseStreaming(): never {
  throw createError({ statusCode: 403, statusMessage: 'Streaming mode', data: { code: streamingCode } })
}

const staffFlags = ['staff', 'setter', 'owner', 'canEdit', 'canViewLiveStandings', 'canRevealResults']

// Whether a streaming browser sees a contest as a participant or as the public. Staff see what a signed-out visitor
// sees, so the broadcast never shows what only staff can read. A check that fails for any reason but a missing or
// refused session is thrown, not answered either way: the caller must not guess a role.
export async function streamingContestView(event: H3Event, id: string): Promise<'participant' | 'public'> {
  if (!hasSession(event)) return 'public'
  try {
    const contest = await privateAPI<Record<string, unknown>>(event, `/my/contests/${id}`, { streaming: true })
    return staffFlags.some(flag => contest[flag] === true) ? 'public' : 'participant'
  } catch (error) {
    if (sessionRefused(error)) return 'public'
    throw error
  }
}
