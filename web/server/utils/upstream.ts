import type { z } from 'zod'

// What the API answered, in the shape the screen expects. Another shape is the API's fault, so the route answers 502.
export function upstream<T extends z.ZodType>(schema: T, value: unknown): z.output<T> {
  const result = schema.safeParse(value)
  if (!result.success) throw createError({ statusCode: 502 })
  return result.data
}
