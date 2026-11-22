// The gateway in front of the API refuses a request with 429 or 503 when more arrive at once than the API runs: its
// answer has no API error code, while the API's own refusals (a rate limit, a database it cannot reach) carry one.
export function refusedUnderLoad(error: unknown) {
  const { response, data } = error as { response?: { status?: number }, data?: { error?: unknown } }
  return (response?.status === 429 || response?.status === 503) && !data?.error
}

// Such a refusal lasts a few tens of milliseconds, so a short wait of a random length, which spreads requests that
// were refused together, is usually enough.
const pause = () => new Promise<void>(resolve => setTimeout(resolve, 100 + Math.random() * 200))

// Sends a request the gateway refused under load once more. Only for requests that are safe to repeat.
export async function retryRefused<T>(request: () => Promise<T>, wait: () => Promise<void> = pause): Promise<T> {
  try { return await request() }
  catch (error) {
    if (!refusedUnderLoad(error)) throw error
    await wait()
    return request()
  }
}
