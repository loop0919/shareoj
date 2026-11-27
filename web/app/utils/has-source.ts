export function hasSource(runtime: string, source: string): boolean {
  return (runtime.replace(/-isolate$/, '') === 'whitespace13' ? source : source.trim()).length > 0
}
// The largest source the API accepts for a submission or a code run, in UTF-8 bytes.
export const sourceLimit = 131072
// What the submission form and the code runner show instead of sending a source the API would refuse; empty when it can go.
export function sourceRefusal(runtime: string, source: string): string {
  return !hasSource(runtime, source) || new TextEncoder().encode(source).length > sourceLimit ? `ソースコードを1〜${sourceLimit.toLocaleString('en-US')}バイトで入力してください。` : ''
}
