import { errorCode } from './api-error'

export function profileError(error: unknown) {
  const e = error as { statusCode?: number }
  const code = errorCode(error)
  // The BFF does not forward invalid_accounts, so it is outside ErrorCode and is compared as plain text. Until it is
  // forwarded, a save the API refuses for the accounts shows the last message below.
  if ((code as string | undefined) === 'invalid_accounts') return '外部アカウントの ID の形式を確認してください。'
  if (code === 'handle_taken') return 'このユーザーIDは使われています。別のIDを入力してください。'
  if (code === 'invalid_profile') return 'このユーザーIDは使えません。別のIDを入力してください。'
  if (code === 'invalid_avatar') return 'アイコンの画像を保存できませんでした。別の画像を選ぶか、アイコンを削除してから保存してください。'
  if (code === 'profile_conflict') return '別の画面でプロフィールが更新されています。再読み込みしてから変更してください。'
  if (e.statusCode === 401) return 'ログインの有効期限が切れました。ログインし直してください。'
  return '保存できませんでした。入力内容を確認し、もう一度お試しください。'
}
