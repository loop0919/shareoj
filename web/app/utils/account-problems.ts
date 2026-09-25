export function accountError(error: unknown) {
  const status = (error as { statusCode?: number, response?: { status?: number } }).statusCode ?? (error as { response?: { status?: number } }).response?.status
  const code = (error as { data?: { data?: { code?: string } } }).data?.data?.code
  if (code === 'featured_ineligible') return '応募には難易度・解説と公開可能な内容が必要です。公開済み、またはコンテスト登録済みの問題は応募できません。'
  if (code === 'featured_limit') return '応募できる問題は作者ごとに3件までです。応募中の問題を取り下げてからお試しください。'
  if (code === 'contest_problem_locked') return 'コンテストに登録された問題は公開・削除できません。公開はコンテスト終了後に自動で行われます。'
  if (status === 401) return 'ログインの有効期限が切れました。別のタブでログインし直してから、もう一度保存してください。'
  if (status === 409) return '別の画面で更新されています。入力内容をコピーしてからページを再読み込みし、変更を確認してください。'
  if (status === 404) return '問題が見つからないか、アクセスできません。'
  return 'サーバーに接続できないか、処理に失敗しました。入力内容を保持したまま、もう一度お試しください。'
}
