import type { TestFile } from './problem-draft'
import { contestProblemPath } from './contest'
import { shortId } from '../utils/short-id'

export type TextPreview = { text: string, truncated: boolean }
export type SampleDetails = { input: TextPreview, expectedOutput: TextPreview, actualOutput: TextPreview }

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
  result: { cpuTimeMs?: number, memoryBytes?: number, interactive?: boolean, verdict: string, passed: number, total: number, compileLog?: string, checkerLog?: string, cases?: { checkerLog?: TextPreview, sampleDetails?: SampleDetails, name: string, verdict: string, output?: string, outputFile?: TestFile, cpuTimeMs?: number, wallTimeMs?: number, memoryBytes?: number }[] } | null
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
