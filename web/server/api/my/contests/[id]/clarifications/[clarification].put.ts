import { contestAction, contestParam, uuidPattern } from '../../../../../utils/private-contest'

export default defineEventHandler(event => contestAction(event, () => `clarifications/${contestParam(event, 'clarification', uuidPattern)}`, 32 * 1024))
