// Accept only known local pages when resuming after authentication.
export function loginDestination(value: unknown) {
  return typeof value === 'string' && /^\/(my(?:\/(?:problems|settings|posts|tester-invitations\/[A-Z2-7]{32}))?|(?:problems|blog)\/new|problems|contests\/[a-f0-9-]{36}(?:\/problems\/[a-f0-9-]{36})?)(\?.*)?$/.test(value) ? value : '/my'
}
