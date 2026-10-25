import type { TextPreview } from './submission'

export type CodeRunResult = {
  verdict: 'AC' | 'RE' | 'TLE' | 'MLE' | 'OLE' | 'CE' | 'JE'
  expired?: true
  exitCode?: number
  cpuTimeMs?: number
  wallTimeMs?: number
  memoryBytes?: number
  stdout?: TextPreview
  stderr?: TextPreview
  compileLog?: string
}

export type CodeRun = {
  id: string
  status: 'QUEUED' | 'RUNNING' | 'DONE'
  result: CodeRunResult | null
  createdAt: string
}

// The largest standard input the API accepts, in UTF-8 bytes (1 MiB).
export const codeRunInputLimit = 1048576
