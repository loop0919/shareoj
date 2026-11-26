import { contestAction } from '../../../../../../utils/private-contest'
import { handlePattern, routeParam } from '../../../../../../utils/route-param'
// Before the start a member removes another member, or their own handle to leave.
export default defineEventHandler(event => contestAction(event, () => `team/members/${routeParam(event, 'handle', handlePattern)}`))
