// Venue passphrases (ADR 0024): after NFKC and trimming, 4 to 64 letters, digits and underscores, compared with case.
export const venuePassphraseRule = '英数字とアンダースコア（_）で4〜64文字'
export const venuePassphraseMessage = '会場の合言葉は、英数字とアンダースコア（_）の4〜64文字で入力してください。'
// The server trims what Go counts as white space, and U+FEFF. JavaScript's \s has U+FEFF but lacks U+0085, so the
// two agree on every value only with it added.
const trimmed = (value: string) => value.normalize('NFKC').replace(/^[\s\u0085]+|[\s\u0085]+$/g, '')
// Spaces alone are empty to the server, so an optional field joins open with them.
export const venuePassphraseBlank = (value: string) => trimmed(value) === ''
export const venuePassphraseValid = (value: string) => /^[A-Za-z0-9_]{4,64}$/.test(trimmed(value))
