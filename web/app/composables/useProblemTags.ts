// Problem tags can give away the solution, so each browser chooses whether to show them.
// A cookie, like the theme, lets the server render the choice without a flash of tags.
export function useProblemTags() {
  const saved = useCookie<string>('openoj-problem-tags', { default: () => 'show', maxAge: 60 * 60 * 24 * 365, sameSite: 'lax', path: '/' })
  return computed<boolean>({
    get: () => saved.value !== 'hide',
    set: value => { saved.value = value ? 'show' : 'hide' },
  })
}
