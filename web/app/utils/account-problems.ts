export function accountError(error: unknown) {
  const status = (error as { statusCode?: number, response?: { status?: number } }).statusCode ?? (error as { response?: { status?: number } }).response?.status
  const detail = (error as { data?: { data?: { code?: string, retryAfter?: number } } }).data?.data
  const code = detail?.code
  if (code === 'creation_quota_exceeded') return detail?.retryAfter
    ? `新規作成の枠を使い切りました。約${Math.ceil(detail.retryAfter / 60)}分後に1件分回復します。入力内容はこの画面に残っています。既存の内容は引き続き編集できます。`
    : '新規作成の枠を使い切りました。枠は2時間に1件分回復します。入力内容はこの画面に残っています。既存の内容は引き続き編集できます。'
  if (code === 'featured_ineligible') return '応募には難易度・解説とテストケース（1件以上）、公開できる内容が必要です。足りない項目は「自分の問題」の状態で確認できます。公開済み、またはコンテスト登録済みの問題は応募できません。'
  if (code === 'featured_limit') return '応募できる問題は作者ごとに3件までです。応募中の問題を取り下げてからお試しください。'
  if (code === 'contest_problem_locked') return 'コンテストに登録された問題は公開・削除できません。公開はコンテスト終了後に自動で行われます。'
  if (status === 401) return 'ログインの有効期限が切れました。別のタブでログインし直してから、もう一度保存してください。'
  if (status === 409) return '別の画面で更新されています。入力内容をコピーしてからページを再読み込みし、変更を確認してください。'
  if (status === 404) return '問題が見つからないか、アクセスできません。'
  return 'サーバーに接続できないか、処理に失敗しました。入力内容を保持したまま、もう一度お試しください。'
}
