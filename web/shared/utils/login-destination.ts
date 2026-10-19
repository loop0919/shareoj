// Accept only known local pages when resuming after authentication.
export function loginDestination(value: unknown) {
  return typeof value === 'string' && /^\/(my(?:\/(?:problems|settings|posts|(?:tester|contest)-invitations\/[A-Z2-7]{32}))?|(?:problems|blog)\/new|problems|contests\/[0-9A-Za-z]{22}(?:\/(?:[A-Z]{1,3}|team))?)(\?.*)?$/.test(value) ? value : '/my'
}
