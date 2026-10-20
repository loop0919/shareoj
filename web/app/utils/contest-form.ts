import type { Contest, ContestDraft } from '~~/shared/types/contest'

export type ParticipationForm = {
  teamMode: 'individual' | 'team', teamSizeOn: boolean, maxTeamSize: number | string, onsiteOn: boolean,
  // Setters only: drafts return it as typed, and a published contest reads it from the setters' venue route.
  passphrase: string,
}
// Number inputs hold '' while empty; drafts save that as null.
export type ContestForm = {
  title: string, description: string, startsAt: string, durationMinutes: number | string, penaltyMinutes: number | string,
  // The length of the standings freeze (ADR 0023), used only while the switch is on.
  freezeOn: boolean, freezeMinutes: number | string,
} & ParticipationForm

// A new object on each call: the editor makes it reactive, and reactive() writes through to the object it wraps.
export function initialContestForm(): ContestForm {
  return {
    title: '', description: '<!-- ここにコンテストの概要を記載 -->\n', startsAt: '', durationMinutes: 120, penaltyMinutes: 5, freezeOn: false, freezeMinutes: '',
    ...participationForm({}),
  }
}

function localDate(value: string) {
  const date = new Date(value)
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16)
}

export function participationForm(s: Pick<ContestDraft, 'teamMode' | 'maxTeamSize' | 'onsiteDivision'>, venue?: string | null): ParticipationForm {
  return { teamMode: s.teamMode ? 'team' : 'individual', teamSizeOn: s.maxTeamSize != null, maxTeamSize: s.maxTeamSize ?? '', onsiteOn: !!s.onsiteDivision, passphrase: venue ?? '' }
}

export function formFromDraft(d: ContestDraft): ContestForm {
  return {
    title: d.title, description: d.description, startsAt: d.startsAt ? localDate(d.startsAt) : '',
    durationMinutes: d.durationMinutes ?? '', penaltyMinutes: d.penaltyMinutes ?? '', freezeOn: d.freezeMinutes != null, freezeMinutes: d.freezeMinutes ?? '',
    ...participationForm(d, d.onsitePassphrase),
  }
}

// Everything but the participation, which a published contest reads from the venue route.
export function formFromContest(c: Contest): Omit<ContestForm, keyof ParticipationForm> {
  return {
    title: c.title, description: c.description, startsAt: localDate(c.startsAt),
    durationMinutes: Math.round((Date.parse(c.endsAt) - Date.parse(c.startsAt)) / 60000),
    penaltyMinutes: c.penaltyMinutes, freezeOn: c.freezeMinutes != null, freezeMinutes: c.freezeMinutes ?? '',
  }
}
