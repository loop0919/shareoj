import type { ForwardedCode, RetryAfterCode } from '~~/shared/utils/error-codes'

// The codes the BFF lets through to the screen. A message keyed by any other code, or a comparison with one, fails the
// type check: the BFF would drop that code, and the screen would show its generic message.
export type ErrorCode = ForwardedCode | RetryAfterCode

export function errorCode(error: unknown) {
  return (error as { data?: { data?: { code?: ErrorCode } } }).data?.data?.code
}
