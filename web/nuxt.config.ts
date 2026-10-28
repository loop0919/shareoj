import process from 'node:process'
import { fileURLToPath } from 'node:url'
import { copyFileSync, mkdirSync } from 'node:fs'
import { docPath, legacyDocPaths } from './shared/docs'

// Stage the WASM outside node_modules so Nitro's package aliases do not rewrite the raw asset path.
const resvgAssets = fileURLToPath(new URL('./.build/og-renderer/', import.meta.url))
mkdirSync(resvgAssets, { recursive: true })
copyFileSync(fileURLToPath(new URL('./node_modules/@resvg/resvg-wasm/index_bg.wasm', import.meta.url)), `${resvgAssets}/index_bg.wasm`)

export default defineNuxtConfig({
  compatibilityDate: '2026-09-08',
  ssr: true,
  nitro: {
    serveStatic: true,
    // ponytail: per-instance cache; use shared storage if duplicate fetches across instances become a problem.
    storage: { cache: { driver: 'lru-cache', max: 1000, ttl: 300_000 } },
    serverAssets: [{ baseName: 'resvg', dir: resvgAssets, pattern: '*.wasm' }],
    ...(process.env.OPENOJ_LAMBDA_BUILD === '1' ? { preset: 'aws-lambda', output: { dir: '.output-lambda' } } : {}),
  },
  devtools: { enabled: false },
  // Bun reports `__esModule` on ES module namespaces, so vite-node's interop in `nuxt dev`
  // swaps zod's exports for its default (the `z` namespace) and `z` goes missing. Let Vite
  // transform zod instead. Builds bundle zod and are unaffected.
  $development: { vite: { ssr: { noExternal: ['zod'] } } },
  routeRules: Object.fromEntries(Object.entries(legacyDocPaths).map(([path, slug]) => [path, { redirect: { to: docPath(slug), statusCode: 301 } }])),
  css: ['katex/dist/katex.min.css', '~/assets/css/main.css', '~/assets/css/editor.css', '~/assets/css/blog.css'],
  runtimeConfig: {
    apiBaseUrl: 'http://127.0.0.1:8080',
    cognitoDomain: '',
    cognitoClientId: '',
    cognitoClientSecret: '',
    public: { siteUrl: 'http://localhost:3000' },
  },
  app: {
    head: {
      htmlAttrs: { lang: 'ja' },
      title: 'ShareOJ — Share Online Judge',
      link: [
        { rel: 'icon', href: '/favicon.ico', sizes: '16x16 32x32 48x48' },
        { rel: 'icon', type: 'image/svg+xml', href: '/favicon.svg', sizes: 'any' },
        { rel: 'apple-touch-icon', href: '/apple-touch-icon.png', sizes: '180x180' },
      ],
      meta: [
        { name: 'color-scheme', content: 'light dark' },
        { property: 'og:site_name', content: 'ShareOJ' },
      ],
    },
  },
})
