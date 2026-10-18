import { contestAction } from '../../../../../utils/private-contest'
// The viewer's team with its join code, which only members see.
export default defineEventHandler(event => contestAction(event, 'team'))
