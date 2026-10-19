import { contestAction, contestParam, handlePattern } from '../../../../../../utils/private-contest'
// A setter moves an open participant in no team, in an individual contest or alone in a team contest, onsite; never
// back to open (ADR 0024).
export default defineEventHandler(event => contestAction(event, () => `participants/${contestParam(event, 'handle', handlePattern)}/onsite`, 256))
