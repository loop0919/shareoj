import { marginPercentSchema, type ErrorJudge } from '~~/shared/types/problem-draft'

const modeLabels: Record<ErrorJudge['mode'], string> = { absolute: '絶対誤差', relative: '相対誤差', either: '絶対誤差または相対誤差' }

export function errorJudgeModeLabel(mode: ErrorJudge['mode']): string {
  return modeLabels[mode]
}

// A margin is a percentage of ε from 0 to 100 with at most six decimals.
export function validMargin(text: string): boolean {
  return marginPercentSchema.safeParse(text).success
}

// The factor 1 + m/100 by which the margin m widens ε, exact and without trailing zeros. Dividing by 100 moves the
// decimal point two places left, so the digits of m become the decimals of the factor; only m = 100 reaches the units.
export function thresholdFactor(marginPercent: string): string {
  if (!validMargin(marginPercent)) throw new RangeError(`invalid margin: ${marginPercent}`)
  const [whole = '', fraction = ''] = marginPercent.split('.')
  const digits = whole.padStart(3, '0')
  if (digits.startsWith('1')) return '2'
  return `1.${digits.slice(1)}${fraction}`.replace(/\.?0+$/, '')
}

export function epsilonExponent(epsilon: string): number {
  const match = /^1e-(\d+)$/.exec(epsilon)
  if (!match) throw new RangeError(`invalid epsilon: ${epsilon}`)
  return -Number(match[1])
}
