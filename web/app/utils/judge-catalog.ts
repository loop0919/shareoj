// No runtime can be picked while the judge is in maintenance or its state could not be fetched.
export function usableRuntimes<T>(catalog: { items?: T[], maintenance?: boolean } | null | undefined, error: unknown): T[] {
  return error || catalog?.maintenance ? [] : catalog?.items ?? []
}

// New judge code starts in C++23 (GCC) unless only others are offered; with none it keeps that name, shown as unavailable.
export function preferredRuntime(items: { id: string }[]): string {
  return items.find(item => item.id === 'cpp23-gcc')?.id ?? items[0]?.id ?? 'cpp23-gcc'
}

// Text only prints its source, so it cannot be a checker, interactor, aggregator or test data generator.
export function judgeCodeRuntimes<T extends { id: string }>(items: T[]): T[] {
  return items.filter(item => item.id !== 'text')
}
