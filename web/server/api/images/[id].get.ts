import { hasSession } from '../../utils/private-session'
import { privateAPI } from '../../utils/private-api'
import { privateHeaders } from '../../utils/private-request'

// The API stores only PNG and JPEG. Tell them apart by the data, not by Blob.type:
// Bun's Response.blob() sometimes drops the Content-Type the API sent.
const signatures = [
  ['image/png', Buffer.from('89504e470d0a1a0a', 'hex')],
  ['image/jpeg', Buffer.from('ffd8ff', 'hex')],
] as const

export default defineEventHandler(async (event) => {
  privateHeaders(event)
  const id = getRouterParam(event, 'id') ?? ''
  if (!/^[a-f0-9-]{36}$/.test(id)) throw createError({ statusCode: 404 })
  let image: Blob
  try {
    image = await privateAPI<Blob>(event, `${hasSession(event) ? '/my' : ''}/images/${id}`)
  } catch (error) {
    // An expired session must not prevent reading a publicly visible image.
    const failure = error as { statusCode?: number, data?: { code?: string } }
    if (failure.statusCode !== 401 && failure.data?.code !== 'profile_required') throw error
    image = await privateAPI<Blob>(event, `/images/${id}`, { token: '' })
  }
  const data = Buffer.from(await image.arrayBuffer())
  const type = signatures.find(([, signature]) => data.subarray(0, signature.length).equals(signature))?.[0]
  if (!type) throw createError({ statusCode: 502 })
  setResponseHeader(event, 'Content-Type', type)
  setResponseHeader(event, 'X-Content-Type-Options', 'nosniff')
  return data
})
