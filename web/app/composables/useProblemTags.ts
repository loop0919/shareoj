// Problem tags can give away the solution, so they stay hidden until the viewer turns them on, from the problem
// list or the settings. The list's switch works signed out too. A cookie, like the theme, lets the server render
// the choice without a flash of tags.
export function useProblemTags() {
  const saved = useCookie<string>('openoj-problem-tags', { default: () => 'hide', maxAge: 60 * 60 * 24 * 365, sameSite: 'lax', path: '/' })
  return computed<boolean>({
    get: () => saved.value === 'show',
    set: value => { saved.value = value ? 'show' : 'hide' },
  })
}
