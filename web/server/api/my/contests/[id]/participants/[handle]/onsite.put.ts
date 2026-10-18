import { contestAction, contestParam, handlePattern } from '../../../../../../utils/private-contest'
// A setter moves a participant of an individual contest between onsite and open.
export default defineEventHandler(event => contestAction(event, () => `participants/${contestParam(event, 'handle', handlePattern)}/onsite`, 256))
