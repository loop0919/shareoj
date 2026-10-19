// Scores travel in hundredths so that sums stay exact (ADR 0025). Whole scores read as integers, others with two
// decimals, like the standings' scoreText.
export function formatScore(hundredths: number) {
  const whole = Math.trunc(hundredths / 100), rest = Math.abs(hundredths % 100)
  return rest ? `${whole}.${String(rest).padStart(2, '0')}` : String(whole)
}

// A case's own score is a raw integer up to 10^18. JSON numbers past 2^53 arrive rounded, so those read as approximate.
export function formatCaseScore(score: number) {
  return Number.isSafeInteger(score) ? String(score) : `約${new Intl.NumberFormat('en-US', { maximumSignificantDigits: 15, useGrouping: false }).format(score)}`
}
