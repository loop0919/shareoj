import { expect, test } from '@playwright/test'
import type { Contest, ContestDraft } from '../shared/types/contest'
import { formFromContest, formFromDraft, initialContestForm, participationForm } from '../app/utils/contest-form'
import { fromJapanInput } from '../app/utils/japan-time'

const draft = (fields: Partial<ContestDraft>): ContestDraft => ({ title: '下書き', description: '', startsAt: null, durationMinutes: null, penaltyMinutes: null, problems: [], ...fields })
const noParticipation = { teamMode: 'individual', teamSizeOn: false, maxTeamSize: '', onsiteOn: false, passphrase: '' }

test('a new contest form starts from the editor defaults, a fresh object each time', () => {
  expect(initialContestForm()).toEqual({ title: '', description: '<!-- ここにコンテストの概要を記載 -->\n', startsAt: '', durationMinutes: 120, penaltyMinutes: 5, freezeOn: false, freezeMinutes: '', ...noParticipation })
  expect(initialContestForm()).not.toBe(initialContestForm())
})

test('a draft fills empty inputs for null and turns a switch on for any value, 0 included', () => {
  expect(formFromDraft(draft({}))).toEqual({ title: '下書き', description: '', startsAt: '', durationMinutes: '', penaltyMinutes: '', freezeOn: false, freezeMinutes: '', ...noParticipation })
  expect(formFromDraft(draft({ durationMinutes: 90, penaltyMinutes: 0, freezeMinutes: 0, teamMode: true, maxTeamSize: 0, onsiteDivision: true, onsitePassphrase: 'Venue_2026' })))
    .toEqual({ title: '下書き', description: '', startsAt: '', durationMinutes: 90, penaltyMinutes: 0, freezeOn: true, freezeMinutes: 0, teamMode: 'team', teamSizeOn: true, maxTeamSize: 0, onsiteOn: true, passphrase: 'Venue_2026' })
})

test('a draft start reads as Japan time for the date input', () => {
  expect(formFromDraft(draft({ startsAt: '2099-10-01T03:04:00Z' })).startsAt).toBe('2099-10-01T12:04')
  // 15:30 in UTC is already the next day in Japan.
  expect(formFromDraft(draft({ startsAt: '2099-09-30T15:30:00Z' })).startsAt).toBe('2099-10-01T00:30')
})

test('a typed start reads as Japan time, and an empty input as no time', () => {
  expect(fromJapanInput('2099-10-01T09:00').toISOString()).toBe('2099-10-01T00:00:00.000Z')
  expect(fromJapanInput('').getTime()).toBeNaN()
})

test('participation settings read apart from the team mode and the onsite division', () => {
  expect(participationForm({ teamMode: false, maxTeamSize: 4, onsiteDivision: false }, 'keep_me')).toEqual({ teamMode: 'individual', teamSizeOn: true, maxTeamSize: 4, onsiteOn: false, passphrase: 'keep_me' })
  expect(participationForm({ teamMode: true, maxTeamSize: null, onsiteDivision: true }, undefined)).toEqual({ teamMode: 'team', teamSizeOn: false, maxTeamSize: '', onsiteOn: true, passphrase: '' })
  expect(participationForm({})).toEqual(noParticipation)
})

test('a published contest reads its length in whole minutes and leaves the participation to participationForm', () => {
  const contest = { title: '公開済み', description: '概要', startsAt: '2099-10-01T00:00:00Z', endsAt: '2099-10-01T01:30:29Z', penaltyMinutes: 0, freezeMinutes: null } as Contest
  const form = formFromContest(contest)
  expect(form).toEqual({ title: '公開済み', description: '概要', startsAt: '2099-10-01T09:00', durationMinutes: 90, penaltyMinutes: 0, freezeOn: false, freezeMinutes: '' })
  expect(formFromContest({ ...contest, endsAt: '2099-10-01T01:30:30Z', freezeMinutes: 30 })).toMatchObject({ durationMinutes: 91, freezeOn: true, freezeMinutes: 30 })
})
