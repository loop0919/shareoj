import { contestAction } from '../../../../../utils/private-contest'
// The setters' list of the teams and the participants in no team, with when and how each became onsite.
export default defineEventHandler(event => contestAction(event, 'participants'))
