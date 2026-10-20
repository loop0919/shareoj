// Resume only at a path of this site. Browsers read // and \ as another host, and control characters
// vanish when a URL is parsed or break the Location header, so values holding them go to my page.
export function loginDestination(value: unknown) {
  return typeof value === 'string' && value.startsWith('/') && !value.includes('//') && !value.includes('\\') && !/[\u0000-\u001f\u007f-\u009f]/.test(value) ? value : '/my'
}
