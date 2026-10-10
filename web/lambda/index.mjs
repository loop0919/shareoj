import { handler as nitro } from '../server/index.mjs'

// Nitro does not pass the Lambda event to request handlers, so copy API Gateway's
// connection address into a header, replacing any value the viewer sent.
export function handler(event, context) {
  const headers = Object.fromEntries(Object.entries(event.headers ?? {}).filter(([name]) => name.toLowerCase() !== 'x-shareoj-client-ip'))
  const ip = event.requestContext?.http?.sourceIp
  if (ip) headers['x-shareoj-client-ip'] = ip
  return nitro({ ...event, headers }, context)
}
