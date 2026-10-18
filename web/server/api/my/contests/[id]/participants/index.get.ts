import { contestAction } from '../../../../../utils/private-contest'
// The setters' list of participants, or of teams in a team contest, with when and how each became onsite.
export default defineEventHandler(event => contestAction(event, 'participants'))
