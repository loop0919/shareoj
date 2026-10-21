export function hasSource(runtime: string, source: string): boolean {
  return (runtime.replace(/-isolate$/, '') === 'whitespace13' ? source : source.trim()).length > 0
}
