// The browser holds streaming mode in this cookie; only the value '1' turns it on.
export const streamingCookie = 'openoj-streaming'
// The code in the 403 body that tells the screen a route is closed in streaming mode.
export const streamingCode = 'streaming_mode'
// Pages whose whole content is staff-only or private. A trailing slash never changes the answer.
export function closedInStreaming(path: string): boolean {
  const clean = path.length > 1 ? path.replace(/\/+$/, '') : path
  if (clean === '/my' || clean.startsWith('/my/')) return clean !== '/my/settings' && !/^\/my\/submissions\/[^/]+$/.test(clean)
  return ['/problems/new', '/problems/post', '/featured/apply', '/blog/new', '/onboarding'].includes(clean) || /^\/contests\/[^/]+\/team$/.test(clean)
}
