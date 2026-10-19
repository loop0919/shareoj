// Messages for the team and onsite errors of ADR 0024. Callers pass their own fallback for anything else.
const messages: Record<string, string> = {
  incorrect_team_code: '参加コードが違います。チームのメンバーに確かめてください。',
  incorrect_venue_passphrase: '会場の合言葉が違います。会場で配られた合言葉を確かめてください。',
  team_full: 'このチームは人数の上限に達しているため入れません。',
  team_name_taken: 'このチーム名はほかのチームが使っています。別の名前にしてください。',
  invalid_team_name: 'チーム名は1〜40文字で入力してください。改行や見えない文字は使えません。',
  team_already_joined: 'すでにチームに入っています。ページを再読み込みしてください。',
  contest_team_required: 'チームを作るか、チームに入って参加してください。',
  contest_team_unavailable: 'このコンテストは個人戦です。ページを再読み込みしてください。',
  contest_onsite_unavailable: 'このコンテストにはオンサイト区分がありません。ページを再読み込みしてください。',
  onsite_kept_open: '作成者がオープン参加に変更したため、会場の合言葉ではオンサイト参加にできません。',
  invalid_venue_passphrase: '会場の合言葉は4〜64文字で入力してください。改行や見えない文字は使えません。',
  team_not_found: 'チームが見つかりません。ページを再読み込みしてください。',
  team_member_not_found: 'このメンバーはもうチームにいません。ページを再読み込みしてください。',
  participant_not_found: '参加者が見つかりません。一覧を更新してください。',
}

export function errorCode(error: unknown) {
  return (error as { data?: { data?: { code?: string } } }).data?.data?.code
}

// The wait the server asks for, in the coarsest unit that still tells when to come back.
function wait(seconds?: number) {
  if (!seconds) return 'しばらく待ってから'
  return seconds < 60 ? `${seconds}秒後に` : `約${Math.ceil(seconds / 60)}分後に`
}

export function contestTeamError(error: unknown, fallback: string) {
  const code = errorCode(error)
  if (code === 'contest_attempts_limited') {
    const retryAfter = (error as { data?: { data?: { retryAfter?: number } } }).data?.data?.retryAfter
    return `参加コードか会場の合言葉の誤りが続いたため、一時的に試せません。${wait(retryAfter)}もう一度お試しください。`
  }
  return (code && messages[code]) || fallback
}
