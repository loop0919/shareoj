import { z } from 'zod'

const slotMetadata = z.object({
  slot: z.enum(['easy', 'hard']),
  kind: z.enum(['new', 'revival', 'missing']),
  difficulty: z.number().int().min(1).max(10).nullable(),
  writer: z.string(),
  testers: z.array(z.string()),
})
const roundSchema = z.object({
  scheduledAt: z.string().datetime({ offset: true }),
  slots: z.array(slotMetadata.extend({
    problemId: z.string(), title: z.string(),
    revealAt: z.string().datetime({ offset: true }), editorialHidden: z.boolean(),
  })),
})
export const featuredPageSchema = z.object({
  nextAt: z.string().datetime({ offset: true }),
  hasMore: z.boolean(),
  waiting: z.object({ easy: z.number().int().nonnegative(), hard: z.number().int().nonnegative() }),
  nextSlots: z.array(slotMetadata),
  current: roundSchema.nullable(),
  items: z.array(roundSchema),
})
export type FeaturedPage = z.infer<typeof featuredPageSchema>
export const featuredApplicationsSchema = z.object({ items: z.array(z.object({
  problemId: z.string().uuid(), title: z.string(), preference: z.enum(['soon', 'later']),
  enteredAt: z.string().datetime({ offset: true }),
})) })
export type FeaturedApplication = z.infer<typeof featuredApplicationsSchema>['items'][number]
