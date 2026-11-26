// Date inputs hold Japan time whatever the viewer's zone; Japan has no daylight saving, so the offset is fixed.
export function japanInput(at: string): string {
  return new Date(new Date(at).getTime() + 9 * 60 * 60 * 1000).toISOString().slice(0, 16)
}
// Reads a date input as Japan time. An empty input gives an Invalid Date.
export function fromJapanInput(value: string): Date {
  return new Date(`${value}:00+09:00`)
}
// The lists' update and reservation times: 2026/10/01 05:00 in Japan time whatever the viewer's zone.
export function japanMinute(at: string): string {
  return new Date(at).toLocaleString('ja-JP', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', timeZone: 'Asia/Tokyo' })
}
// The lists' publication dates: 2026/10/1 in Japan time.
export function japanDate(at: string): string {
  return new Date(at).toLocaleDateString('ja-JP', { timeZone: 'Asia/Tokyo' })
}
