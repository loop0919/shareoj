import { contestAction, contestParam, uuidPattern } from '../../../../../../utils/private-contest'
// A setter renames a team until the results are final.
export default defineEventHandler(event => contestAction(event, () => `teams/${contestParam(event, 'team', uuidPattern)}/name`, 2 << 10))
