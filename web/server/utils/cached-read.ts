// Keeps the last answer of a slow or limited source inside this server process. A value younger than `freshMs` is
// answered without reading. After that the next reader reads again, and readers that arrive meanwhile wait for that
// read. When the read fails, the last value is answered while it is younger than `staleMs`; a failure is never kept.
export function cachedRead<T>(freshMs: number, staleMs: number, now: () => number = () => Date.now()) {
  let last: { value: T, at: number } | undefined
  let reading: Promise<T> | undefined
  return async (read: () => Promise<T>): Promise<T> => {
    if (last && now() - last.at < freshMs) return last.value
    reading ??= read().then(value => { last = { value, at: now() }; return value }).finally(() => { reading = undefined })
    try { return await reading }
    catch (error) {
      if (last && now() - last.at < staleMs) return last.value
      throw error
    }
  }
}
