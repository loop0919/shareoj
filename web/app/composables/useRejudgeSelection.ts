import type { Submission } from '~~/shared/types/submission'

// The rows chosen for a rejudge in a list of one problem's submissions, shown by SubmissionTable's select column. Only
// finished judgements can be rejudged, and the selection belongs to the visible page: items gives that page's rows.
// rejudge is useRejudge's, and reload reads the list again once the rejudge is queued.
export function useRejudgeSelection(items: () => Submission[] | undefined, rejudge: (submissionIds: string[]) => Promise<number>, reload: () => Promise<unknown>) {
  const selected = ref<string[]>([])
  const selectable = computed(() => items()?.filter(item => item.status === 'DONE').map(item => item.id) ?? [])
  watch(selectable, ids => { selected.value = selected.value.filter(id => ids.includes(id)) })
  async function rejudgeSelected() {
    const count = selected.value.length
    if (!count || !window.confirm(`選択した${count}件の提出をリジャッジしますか？\n保存済みの問題の内容で採点し直します。`)) return
    if (await rejudge([...selected.value])) { selected.value = []; await reload() }
  }
  return { selected, selectable, rejudgeSelected }
}
