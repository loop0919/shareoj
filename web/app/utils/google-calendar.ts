// Google Calendar's event form, filled in. Times go out in UTC, so the calendar shows them in its owner's zone.
const stamp = (iso: string) => new Date(iso).toISOString().replace(/[-:]|\.\d{3}/g, '')

export function googleCalendarUrl(event: { title: string, startsAt: string, endsAt: string, url: string }) {
  return `https://calendar.google.com/calendar/render?${new URLSearchParams({
    action: 'TEMPLATE',
    text: event.title,
    dates: `${stamp(event.startsAt)}/${stamp(event.endsAt)}`,
    details: event.url,
  })}`
}
