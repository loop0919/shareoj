import { closedInStreaming, streamingCode } from '~~/shared/utils/streaming-mode'

// Closed pages never load: the navigation stops with an error the error screen turns into the closed screen. It runs
// on the server and the client alike, so a direct load and an in-site move end on the same screen.
export default defineNuxtRouteMiddleware(to => {
  const { streaming } = useStreamingMode()
  if (streaming.value && closedInStreaming(String(to.name ?? ''))) throw createError({ statusCode: 403, statusMessage: 'Streaming mode', data: { code: streamingCode }, fatal: true })
})
