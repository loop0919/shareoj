import { contestAction } from '../../../../utils/private-contest'
// Setters read the venue passphrase, and replace it with PUT until the end.
export default defineEventHandler(event => {
  if (event.method !== 'GET' && event.method !== 'PUT') throw createError({ statusCode: 405 })
  return contestAction(event, 'onsite', event.method === 'PUT' ? 4 << 10 : 0)
})
