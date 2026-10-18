import { contestAction } from '../../../../../utils/private-contest'
// A member renames the team.
export default defineEventHandler(event => contestAction(event, 'team/name', 2 << 10))
