// The オンサイトのみ表示 switch for the standings and the submission lists, kept in ?scope=onsite so a shared link keeps the
// filter (ADR 0024). It applies only while available() holds, for example in a contest with an onsite division.
export function useOnsiteScope(available: () => boolean) {
  const route = useRoute()
  const router = useRouter()
  const onsite = computed({
    get: () => available() && route.query.scope === 'onsite',
    set: (on: boolean) => { void router.replace({ query: { ...route.query, scope: on ? 'onsite' : undefined } }) },
  })
  return { onsite }
}
