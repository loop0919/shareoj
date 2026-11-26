import { contestAction } from '../../../../../utils/private-contest'
import { routeParam, uuidPattern } from '../../../../../utils/route-param'

export default defineEventHandler(event => contestAction(event, () => `clarifications/${routeParam(event, 'clarification', uuidPattern)}`, 32 * 1024))
