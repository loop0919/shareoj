import type { Submission } from '~~/shared/types/submission'

// How a submission list keeps up with a rejudge asked for from it, for its 3-second polling (active).
// Rows in view that are being judged keep the polling going, as they always have. A rejudged submission is judged again,
// so a list narrowed by conditions may lose it while it is judged, and may not get it back where it was. So after a
// rejudge asked for here while narrowed() holds, the list is followed until rows read after the rejudge show every
// rejudged submission judged (DONE), and for a minute at most. Without conditions the rows stay in view, and the
// judging rows alone keep the pace.
// items gives the rows on screen. Every read of the list goes through read(), which numbers it as it is sent, so rows
// from a request sent before the rejudge, such as a reload that shared an older request, never end the follow early.
export function useRejudgeFollow(options: { items: () => Submission[] | undefined, narrowed: () => boolean }) {
  let asked = 0
  let shown = 0
  let followed: { ids: string[], after: number, until: number } | undefined
  // One read of the list, for useLatestRequest's run: fetch sends it, and apply shows its rows.
  function read<T>(fetch: () => Promise<T>, apply: (value: T) => void) {
    let number = 0
    return {
      fetch: () => { number = ++asked; return fetch() },
      apply: (value: T) => { apply(value); shown = number },
    }
  }
  // The list's rejudge, followed when it queues anything while the list is narrowed.
  function follows(rejudge: (submissionIds: string[]) => Promise<number>) {
    return async (submissionIds: string[]) => {
      const queued = await rejudge(submissionIds)
      if (queued && options.narrowed()) followed = { ids: submissionIds, after: asked, until: Date.now() + 60_000 }
      return queued
    }
  }
  function following() {
    const items = options.items() ?? []
    const judged = (id: string) => items.some(item => item.id === id && item.status === 'DONE')
    if (followed && (Date.now() >= followed.until || (shown > followed.after && followed.ids.every(judged)))) followed = undefined
    return !!followed
  }
  // Whether the list is worth reading every 3 seconds.
  const active = () => !!options.items()?.some(item => item.status !== 'DONE') || following()
  return { read, follows, active }
}
