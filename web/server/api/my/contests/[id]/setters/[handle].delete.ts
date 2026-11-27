import { contestAction } from '../../../../../utils/private-contest'
import { handlePattern, routeParam } from '../../../../../utils/route-param'
// The owner removes a co-setter, or a co-setter removes their own handle to leave.
export default defineEventHandler(event => contestAction(event, () => `setters/${routeParam(event, 'handle', handlePattern)}`))
