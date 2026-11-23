// The browser holds streaming mode in this cookie; only the value '1' turns it on.
export const streamingCookie = 'openoj-streaming'
// The code in the 403 body that tells the screen a route is closed in streaming mode.
export const streamingCode = 'streaming_mode'
// Route names of the pages whose whole content is staff-only or private. The matched route decides, not the URL text, so a
// respelled path (other case, trailing slash, percent-encoding) that still resolves to the page is closed too.
const closedNames = new Set(['problems-new', 'problems-post', 'featured-apply', 'blog-new', 'onboarding', 'contests-id-team'])
const openUnderMy = new Set(['my-settings', 'my-submissions-id'])
export function closedInStreaming(routeName: string): boolean {
  if (routeName === 'my' || routeName.startsWith('my-')) return !openUnderMy.has(routeName)
  return closedNames.has(routeName)
}
