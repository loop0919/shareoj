import type { Contest, JoinBody } from '~~/shared/types/contest'
import { contestTeamError } from '~/utils/contest-team'

// The participation request shared by the contest page and the team page (ADR 0024). A refusal registers nothing, so
// callers keep their inputs for another try and show error.
export function useContestJoin(id: () => string) {
  const saving = ref(false)
  const error = ref('')
  const fallback = '参加登録できませんでした。ログイン状態・開催期間を確認し、再読み込みしてお試しください。'
  async function send(body?: JoinBody) {
    saving.value = true
    error.value = ''
    try {
      // Without a team or a passphrase the request stays empty, as from the pages before them.
      return await $fetch<Contest>(`/api/my/contests/${id()}/participation`, { method: 'POST', ...body && Object.keys(body).length ? { body } : {} })
    } catch (failure) {
      error.value = contestTeamError(failure, fallback)
      return undefined
    } finally { saving.value = false }
  }
  return { saving, error, send }
}

// A venue passphrase typed on the overview before choosing 「チーム参加」 goes along to the team page, in memory only, so
// it is not typed twice.
export const useCarriedPassphrase = (id: string) => useState(`contest-venue:${id}`, () => '')
