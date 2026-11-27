import { contestAction } from '../../../../utils/private-contest'
export default defineEventHandler(event => contestAction(event, 'reveal'))
