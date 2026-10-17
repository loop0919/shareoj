// Rejudging is offered to the problem's author and testers and to operators. The API answers
// 404 to everyone else, so a failed check simply hides the controls.
export function useRejudge(problemId: MaybeRefOrGetter<string>) {
  const { user } = useAccount()
  const allowed = ref(false)
  const pending = ref(0)
  const busy = ref(false)
  const message = ref('')
  async function refresh() {
    const id = toValue(problemId)
    if (!user.value || !id) { allowed.value = false; pending.value = 0; return }
    try {
      pending.value = (await $fetch<{ pending: number }>(`/api/my/problems/${id}/rejudge`)).pending
      allowed.value = true
    } catch { allowed.value = false }
  }
  // Without submissionIds, every finished submission of the problem is rejudged.
  async function rejudge(submissionIds?: string[]) {
    busy.value = true
    message.value = ''
    try {
      const { queued } = await $fetch<{ queued: number }>(`/api/my/problems/${toValue(problemId)}/rejudge`, { method: 'POST', body: submissionIds ? { submissionIds } : {} })
      message.value = queued
        ? `${queued}件の提出をリジャッジしています。新しい提出の採点を優先するため、終わるまで時間がかかることがあります。`
        : 'リジャッジできる提出はありませんでした。採点中の提出と、現在使えない言語の提出は対象外です。'
      await refresh()
      return queued
    } catch (error) {
      const failure = error as { statusCode?: number, data?: { data?: { code?: string } } }
      const code = failure.data?.data?.code
      if (code === 'judge_maintenance') message.value = judgeMaintenanceMessage
      else if (code === 'tests_not_ready') message.value = '今の問題の設定では採点できません。テストケース、判定コード、利用できる言語の設定を確認してください。'
      else if (failure.statusCode === 404) message.value = 'この問題の提出をリジャッジする権限がありません。'
      else message.value = 'リジャッジを依頼できませんでした。時間をおいて、もう一度お試しください。'
      return 0
    } finally { busy.value = false }
  }
  watch([() => user.value?.id, () => toValue(problemId)], refresh)
  onMounted(refresh)
  return { allowed, pending, busy, message, refresh, rejudge }
}
