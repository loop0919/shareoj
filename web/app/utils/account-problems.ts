// The publication call refuses an incomplete draft with the same code as a save, but nothing was being saved then.
export function accountError(error: unknown, action: 'save' | 'publish' = 'save') {
  const status = (error as { statusCode?: number, response?: { status?: number } }).statusCode ?? (error as { response?: { status?: number } }).response?.status
  const detail = (error as { data?: { data?: { code?: string, retryAfter?: number } } }).data?.data
  const code = detail?.code
  if (code === 'creation_quota_exceeded') return detail?.retryAfter
    ? `新規作成の枠を使い切りました。約${Math.ceil(detail.retryAfter / 60)}分後に1件分回復します。入力内容はこの画面に残っています。既存の内容は引き続き編集できます。`
    : '新規作成の枠を使い切りました。枠は2時間に1件分回復します。入力内容はこの画面に残っています。既存の内容は引き続き編集できます。'
  if (code === 'incomplete_problem') return action === 'publish'
    ? '足りない項目があるため公開できません。タイトル・本文・難易度・テストケース（1件以上）・解説と、判定コードと採点の設定を確認してください。公開と予約の状態は変わっていません。'
    : 'タイトル・本文・難易度・テストケース（1件以上）・解説と、判定コードと採点の設定を確認してください。公開中の問題と、結果の確定前のコンテストの問題は、足りない項目があると保存できません。保存済みの内容は変わっていません。'
  if (code === 'problem_scoring_locked') return 'コンテストが始まったため、採点方式とグループの名前・配点・順番は変えられません。ケースの所属、テストケース、集計コードは直せます。'
  if (code === 'invalid_schedule') return '未来の公開日時を指定してください。公開中・コンテスト登録済み・定期便応募中の問題は予約できません。'
  if (code === 'featured_ineligible') return '応募には難易度・解説とテストケース（1件以上）、公開できる内容が必要です。足りない項目は「自分の問題」の状態で確認できます。公開済み、またはコンテスト登録済みの問題は応募できません。'
  if (code === 'featured_limit') return '応募できる問題は作者ごとに3件までです。応募中の問題を取り下げてからお試しください。'
  if (code === 'contest_problem_locked') return 'コンテストに登録された問題は公開・削除できません。公開はコンテスト終了後に自動で行われます。'
  if (code === 'contest_participant_locked') return 'この問題が入っているコンテストに参加登録しているため、結果が確定するまで招待を受けられません。'
  if (code === 'contest_invitation_closed') return 'コンテストの結果が確定したため、作成者に加われません。'
  if (code === 'setter_not_found') return '共同作成者が見つかりません。ページを再読み込みして、今の作成者を確認してください。'
  if (status === 401) return 'ログインの有効期限が切れました。別のタブでログインし直してから、もう一度保存してください。'
  if (status === 409) return '別の画面で更新されています。入力内容をコピーしてからページを再読み込みし、変更を確認してください。'
  if (status === 404) return '問題が見つからないか、アクセスできません。'
  return 'サーバーに接続できないか、処理に失敗しました。入力内容を保持したまま、もう一度お試しください。'
}
