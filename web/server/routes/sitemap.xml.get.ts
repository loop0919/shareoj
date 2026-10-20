import { socialPages } from '~~/shared/social-pages'

export default defineEventHandler(event => {
  const siteUrl = useRuntimeConfig(event).public.siteUrl
  // Stable entry points are the pages with a fixed share image; public content is linked from the catalogue pages.
  const paths = Object.keys(socialPages)
  const urls = paths.map(path => `<url><loc>${new URL(path, siteUrl).href.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')}</loc></url>`)
  setResponseHeader(event, 'Content-Type', 'application/xml; charset=utf-8')
  return `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">${urls.join('')}</urlset>`
})
