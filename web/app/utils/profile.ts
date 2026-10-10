export function profileError(error: unknown) {
  const e = error as { data?: { data?: { code?: string } }, statusCode?: number }
  const code = e.data?.data?.code
  if (code === 'invalid_accounts') return '外部アカウントの ID の形式を確認してください。'
  if (code === 'handle_taken') return 'このユーザーIDは使われています。別のIDを入力してください。'
  if (code === 'invalid_profile') return 'このユーザーIDは使えません。別のIDを入力してください。'
  if (code === 'profile_conflict') return '別の画面でプロフィールが更新されています。再読み込みしてから変更してください。'
  if (e.statusCode === 401) return 'ログインの有効期限が切れました。ログインし直してください。'
  return '保存できませんでした。入力内容を確認し、もう一度お試しください。'
}
