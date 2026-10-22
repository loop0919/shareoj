import { contestAction } from '../../../../../utils/private-contest'

export default defineEventHandler(event => {
  const offset = getQuery(event).offset
  return contestAction(event, `clarifications?offset=${encodeURIComponent(typeof offset === 'string' ? offset : '0')}`)
})
