import type { TestFile } from './problem-draft'
import { contestProblemPath } from './contest'
import { shortId } from '../utils/short-id'

export type TextPreview = { text: string, truncated: boolean }
export type SampleDetails = { input: TextPreview, expectedOutput: TextPreview, actualOutput: TextPreview }

// A partially scored result (ADR 0025): scores in hundredths, and each group with the positions of its cases in
// result.cases. Lists leave out the groups. A case's own score is a raw integer, given only in the aggregate mode.
export type SubmissionScore = { score: number, maxScore: number, groups?: { name: string, score: number, maxScore: number, cases: number[] }[] }

export type Submission = {
  contestId?: string
  problemIndex?: number
  author?: string
  easyTest?: boolean
  id: string
  problemId: string
  problemVersion: number
  problemTitle: string
  runtime: string
  sourceBytes?: number
  source?: string
  status: 'QUEUED' | 'RUNNING' | 'DONE'
  progress?: { phase: 'PREPARING' | 'JUDGING', completed: number, total: number, verdict?: 'WA' | 'TLE' | 'MLE' | 'OLE' | 'RE' } | null
  result: { cpuTimeMs?: number, memoryBytes?: number, interactive?: boolean, verdict: string, passed: number, total: number, compileLog?: string, checkerLog?: string, scoring?: SubmissionScore, cases?: { checkerLog?: TextPreview, sampleDetails?: SampleDetails, name: string, verdict: string, score?: number, output?: string, outputFile?: TestFile, cpuTimeMs?: number, wallTimeMs?: number, memoryBytes?: number }[] } | null
  createdAt: string
  rejudgedAt?: string
}

// Without the contest position, link the older path; its page redirects to the problem's letter.
export function submissionProblemPath(item: Submission, contestId = item.contestId) {
  if (!contestId) return `/problems/${shortId(item.problemId)}`
  return item.problemIndex === undefined || contestId !== item.contestId
    ? `/contests/${shortId(contestId)}/problems/${shortId(item.problemId)}`
    : contestProblemPath(contestId, item.problemIndex)
}
