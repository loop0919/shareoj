import { contestAction } from '../../../../../../utils/private-contest'
import { routeParam, uuidPattern } from '../../../../../../utils/route-param'
// A setter moves an open team onsite; never back to open (ADR 0024).
export default defineEventHandler(event => contestAction(event, () => `teams/${routeParam(event, 'team', uuidPattern)}/onsite`, 256))
