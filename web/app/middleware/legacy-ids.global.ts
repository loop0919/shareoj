import { isUuid, shortId } from '~~/shared/utils/short-id'

// Older links carry UUIDs in the path. The contest problem page resolves its old form itself.
export default defineNuxtRouteMiddleware(to => {
  const segments = to.path.split('/')
  if (!segments.some(isUuid) || /^\/contests\/[^/]+\/problems\/[^/]+$/.test(to.path)) return
  return navigateTo({ path: segments.map(shortId).join('/'), query: to.query, hash: to.hash }, { replace: true, redirectCode: 301 })
})
