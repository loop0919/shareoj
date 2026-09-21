import type { TestFile } from './problem-draft'

export type TextPreview = { text: string, truncated: boolean }
export type SampleDetails = { input: TextPreview, expectedOutput: TextPreview, actualOutput: TextPreview }

export type Submission = {
  contestId?: string
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
}
