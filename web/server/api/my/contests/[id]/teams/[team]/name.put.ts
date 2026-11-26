import { contestAction } from '../../../../../../utils/private-contest'
import { routeParam, uuidPattern } from '../../../../../../utils/route-param'
// A setter renames a team until the results are final.
export default defineEventHandler(event => contestAction(event, () => `teams/${routeParam(event, 'team', uuidPattern)}/name`, 2 << 10))
