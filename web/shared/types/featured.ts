import { z } from 'zod'

export const featuredPageSchema = z.object({
  nextAt: z.string().datetime({ offset: true }),
  hasMore: z.boolean(),
  items: z.array(z.object({
    scheduledAt: z.string().datetime({ offset: true }),
    slots: z.array(z.object({
      slot: z.enum(['easy', 'hard']),
      kind: z.enum(['new', 'revival', 'missing']),
      problemId: z.string(),
      title: z.string(),
      difficulty: z.number().int().min(1).max(10).nullable(),
      revealAt: z.string().datetime({ offset: true }),
      editorialHidden: z.boolean(),
    })),
  })),
})
export const featuredApplicationsSchema = z.object({ items: z.array(z.object({
  problemId: z.string().uuid(), title: z.string(), preference: z.enum(['soon', 'later']),
  enteredAt: z.string().datetime({ offset: true }),
})) })
export type FeaturedApplication = z.infer<typeof featuredApplicationsSchema>['items'][number]
