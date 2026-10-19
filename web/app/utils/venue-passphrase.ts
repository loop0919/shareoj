// Venue passphrases (ADR 0024): after NFKC and trimming, 4 to 64 letters, digits and underscores, compared with case.
export const venuePassphraseRule = '英数字とアンダースコア（_）で4〜64文字'
export const venuePassphraseMessage = '会場の合言葉は、英数字とアンダースコア（_）の4〜64文字で入力してください。'
// What the browser checks as typed. Full-width letters, digits and underscores, and spaces around them, are what the
// server folds and trims, so they pass here too.
export const venuePassphrasePattern = '\\s*[A-Za-z0-9_Ａ-Ｚａ-ｚ０-９＿]{4,64}\\s*'
export const venuePassphraseValid = (value: string) => /^[A-Za-z0-9_]{4,64}$/.test(value.normalize('NFKC').trim())
