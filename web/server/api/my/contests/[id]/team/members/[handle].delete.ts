import { contestAction, contestParam, handlePattern } from '../../../../../../utils/private-contest'
// Before the start a member removes another member, or their own handle to leave.
export default defineEventHandler(event => contestAction(event, () => `team/members/${contestParam(event, 'handle', handlePattern)}`))
