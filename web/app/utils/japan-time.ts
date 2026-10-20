// Date inputs hold Japan time whatever the viewer's zone; Japan has no daylight saving, so the offset is fixed.
export function japanInput(at: string): string {
  return new Date(new Date(at).getTime() + 9 * 60 * 60 * 1000).toISOString().slice(0, 16)
}
// Reads a date input as Japan time. An empty input gives an Invalid Date.
export function fromJapanInput(value: string): Date {
  return new Date(`${value}:00+09:00`)
}
