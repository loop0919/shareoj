import { contestAction, contestParam, handlePattern } from '../../../../../utils/private-contest'
// The owner removes a co-setter, or a co-setter removes their own handle to leave.
export default defineEventHandler(event => contestAction(event, () => `setters/${contestParam(event, 'handle', handlePattern)}`))
