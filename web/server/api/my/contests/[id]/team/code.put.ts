import { contestAction } from '../../../../../utils/private-contest'
// A member replaces the join code; the old one stops working.
export default defineEventHandler(event => contestAction(event, 'team/code'))
