import { z } from 'zod'
export const maxTags = 5
export const maxTagLength = 30
export const tagSchema = z.object({ name: z.string().min(1), problemCount: z.number().int().nonnegative(), postCount: z.number().int().nonnegative() })
export const tagListSchema = z.object({ items: z.array(tagSchema) })
export type Tag = z.infer<typeof tagSchema>

// Same rule as the API: NFKC folds full-width letters and half-width kana, and runs of spaces become one.
export function normalizeTag(value: string) {
  return value.normalize('NFKC').trim().split(/\s+/).filter(Boolean).join(' ')
}
// Tags that differ only in letter case are the same tag.
export const tagKey = (value: string) => normalizeTag(value).toLowerCase()

export function tagError(tag: string) {
  if ([...tag].length > maxTagLength) return `タグは${maxTagLength}文字以内にしてください。`
  if (/[,\u0000-\u001f\u007f-\u009f]/.test(tag)) return 'タグにカンマや改行は使えません。'
  return ''
}
