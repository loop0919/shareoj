import { contestAction, contestParam, uuidPattern } from '../../../../../../utils/private-contest'
// A setter moves an open team onsite; never back to open (ADR 0024).
export default defineEventHandler(event => contestAction(event, () => `teams/${contestParam(event, 'team', uuidPattern)}/onsite`, 256))
