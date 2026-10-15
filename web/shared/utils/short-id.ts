// Page URLs carry the 128 bits of a UUID as 22 base62 characters; the API keeps the UUID form.
const alphabet = '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz'
const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/
const shortPattern = /^[0-9A-Za-z]{22}$/
const limit = 1n << 128n

export const isUuid = (value: string) => uuidPattern.test(value)

// Values that are not UUIDs pass through, so non-UUID identifiers keep working.
export function shortId(id: string) {
  if (!isUuid(id)) return id
  let n = BigInt(`0x${id.replaceAll('-', '')}`)
  let out = ''
  for (let i = 0; i < 22; i++) {
    out = alphabet[Number(n % 62n)] + out
    n /= 62n
  }
  return out
}

// Turns a route parameter back into the API's UUID; anything else passes through for the API to reject.
export function fullId(value: unknown) {
  const text = String(value ?? '')
  if (!shortPattern.test(text)) return text
  let n = 0n
  for (const char of text) n = n * 62n + BigInt(alphabet.indexOf(char))
  if (n >= limit) return text
  const hex = n.toString(16).padStart(32, '0')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}
