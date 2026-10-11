// Narrows a loaded list to one status group for StatusFilter; an unknown key in the URL shows everything.
export function useStatusFilter<T, K extends string>(options: readonly { key: K, label: string }[], selected: Ref<string>, entries: () => T[], groupOf: (entry: T) => K) {
  const filter = computed<K | ''>({
    get: () => options.find(option => option.key === selected.value)?.key ?? '',
    set: key => { selected.value = key },
  })
  const groups = computed(() => entries().map(groupOf))
  const counts = computed(() => Object.fromEntries(options.map(option => [option.key, groups.value.filter(group => group === option.key).length])) as Record<K, number>)
  const visible = computed(() => filter.value ? entries().filter((_, index) => groups.value[index] === filter.value) : entries())
  const label = computed(() => options.find(option => option.key === filter.value)?.label ?? '')
  return { filter, counts, visible, label }
}
