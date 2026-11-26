import { contestAction } from '../../../../../../utils/private-contest'
import { handlePattern, routeParam } from '../../../../../../utils/route-param'
// A setter moves an open participant in no team, in an individual contest or alone in a team contest, onsite; never
// back to open (ADR 0024).
export default defineEventHandler(event => contestAction(event, () => `participants/${routeParam(event, 'handle', handlePattern)}/onsite`, 256))
