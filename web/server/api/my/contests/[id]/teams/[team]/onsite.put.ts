import { contestAction, contestParam, uuidPattern } from '../../../../../../utils/private-contest'
// A setter moves a team between onsite and open.
export default defineEventHandler(event => contestAction(event, () => `teams/${contestParam(event, 'team', uuidPattern)}/onsite`, 256))
