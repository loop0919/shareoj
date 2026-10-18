// 全員 or オンサイトのみ for the standings and the submission lists, kept in ?scope=onsite so a shared link keeps the
// filter (ADR 0024). It applies only while available() holds, for example in a contest with an onsite division.
export function useOnsiteScope(available: () => boolean) {
  const route = useRoute()
  const router = useRouter()
  const onsite = computed(() => available() && route.query.scope === 'onsite')
  const scope = computed({
    get: () => onsite.value ? 'onsite' as const : 'all' as const,
    set: (value: 'all' | 'onsite') => { void router.replace({ query: { ...route.query, scope: value === 'onsite' ? 'onsite' : undefined } }) },
  })
  const scopes = [{ value: 'all' as const, label: '全員' }, { value: 'onsite' as const, label: 'オンサイトのみ' }]
  return { onsite, scope, scopes }
}
