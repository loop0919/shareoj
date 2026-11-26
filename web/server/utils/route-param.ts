import type { H3Event } from 'h3'
import { userHandle } from '~~/shared/types/profile'

// The loose pattern for an ID in a route: 36 characters of hex digits and hyphens. isUuid and zod's uuid() are
// stricter and refuse some values this one lets through to the API, so a route keeps the check it has.
export const uuidPattern = /^[a-f0-9-]{36}$/
export const handlePattern = userHandle

// A route parameter that must match the pattern, or the route answers 404.
export function routeParam(event: H3Event, name: string, pattern: RegExp) {
  const value = getRouterParam(event, name) ?? ''
  if (!pattern.test(value)) throw createError({ statusCode: 404 })
  return value
}
