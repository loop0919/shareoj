export interface Clarification {
  id: string
  problemId: string
  question: string
  author: string
  answer: string
  answeredBy: string
  answeredAt: string | null
  publishedAt: string | null
  createdAt: string
  version: number
  canManage: boolean
}

export interface ClarificationList { items: Clarification[], hasMore: boolean }
