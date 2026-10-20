// Preloaded into the account suite's web server, which has no Cognito to exchange a Google code with.
// The token endpoint answers with the code itself as the access token, so a test can pass a fixture API token as the code.
const tokenURL = new URL('/oauth2/token', process.env.NUXT_COGNITO_DOMAIN).href
const originalFetch = globalThis.fetch
globalThis.fetch = (input, init) => String(input) === tokenURL
  ? Promise.resolve(Response.json({ access_token: new URLSearchParams(String(init?.body)).get('code'), token_type: 'Bearer', expires_in: 3600 }))
  : originalFetch(input, init)
