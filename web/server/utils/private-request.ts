import type { H3Event } from 'h3'

export function privateHeaders(event: H3Event) {
  setResponseHeader(event, 'Cache-Control', 'no-store')
  setResponseHeader(event, 'Vary', 'Cookie')
}

export function requireSameOrigin(event: H3Event) {
  const origin = getHeader(event, 'origin')
  const expected = new URL(useRuntimeConfig(event).public.siteUrl).origin
  if (origin !== expected) throw createError({ statusCode: 403, statusMessage: 'Invalid origin' })
}

// Read incrementally so chunked requests have the same limit as Content-Length requests.
export async function limitedJSON(event: H3Event, limit: number) {
  if (getHeader(event, 'content-type')?.split(';')[0]?.trim() !== 'application/json') {
    throw createError({ statusCode: 415, statusMessage: 'JSON required' })
  }
  const declaredLength = Number(getHeader(event, 'content-length'))
  if (declaredLength > limit) throw createError({ statusCode: 413, statusMessage: 'Request too large' })
  // Nitro's Lambda adapter provides a buffered body, not a Node readable stream.
  if ('body' in event.node.req || 'rawBody' in event.node.req || event.web?.request) {
    const body = await readRawBody(event, false)
    if (body && body.length > limit) throw createError({ statusCode: 413, statusMessage: 'Request too large' })
    try { return JSON.parse(body?.toString('utf8') ?? '') as unknown }
    catch { throw createError({ statusCode: 400, statusMessage: 'Invalid JSON' }) }
  }
  const body = await new Promise<Buffer>((resolve, reject) => {
    const chunks: Buffer[] = []
    let length = 0
    event.node.req.on('data', chunk => {
      length += chunk.length
      if (length > limit) {
        chunks.length = 0
        reject(createError({ statusCode: 413, statusMessage: 'Request too large' }))
        return // Drain subsequent chunks without retaining them or closing the response socket.
      }
      chunks.push(Buffer.from(chunk))
    })
    event.node.req.once('end', () => resolve(Buffer.concat(chunks)))
    event.node.req.once('error', reject)
    event.node.req.once('aborted', () => reject(createError({ statusCode: 400, statusMessage: 'Request aborted' })))
  })
  try { return JSON.parse(body.toString('utf8')) as unknown }
  catch { throw createError({ statusCode: 400, statusMessage: 'Invalid JSON' }) }
}

// A JSON object that clients may leave out: a request without a Content-Type or a length has no body.
export async function optionalJSON(event: H3Event, limit: number) {
  if (!getHeader(event, 'content-type') && !Number(getHeader(event, 'content-length')) && !getHeader(event, 'transfer-encoding')) return undefined
  const body = await limitedJSON(event, limit)
  if (!body || typeof body !== 'object' || Array.isArray(body)) throw createError({ statusCode: 400, statusMessage: 'Invalid JSON' })
  return body as Record<string, unknown>
}
