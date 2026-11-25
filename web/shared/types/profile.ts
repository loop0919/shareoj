import { z } from 'zod'
export const accountsSchema = z.object({
  x: z.string().default(''), atcoder: z.string().default(''),
  codeforces: z.string().default(''), yukicoder: z.string().default(''),
})
export const accountServices = [
  { key: 'x', label: 'X', url: 'https://x.com/', pattern: '@?[A-Za-z0-9_]{1,15}', max: 16 },
  { key: 'atcoder', label: 'AtCoder', url: 'https://atcoder.jp/users/', pattern: '[A-Za-z0-9_]{1,16}', max: 16 },
  { key: 'codeforces', label: 'Codeforces', url: 'https://codeforces.com/profile/', pattern: '[A-Za-z0-9_.\\-]{3,24}', max: 24 },
  { key: 'yukicoder', label: 'yukicoder', url: 'https://yukicoder.me/users/', pattern: '[0-9]{1,20}', max: 20 },
] as const
export const userHandle = /^[a-z][a-z0-9_]{2,19}$/
// One request for icons carries at most this many handles; the API refuses more, so that an answer full of icons
// stays small enough for a Lambda to return.
export const iconBatchSize = 20
export const profileSchema = z.object({
  accounts: accountsSchema.prefault({}),
  handle: z.string().regex(userHandle),
  avatar: z.string().max(180000).refine(v => v === '' || /^data:image\/png;base64,[A-Za-z0-9+/=]+$/.test(v)),
  version: z.number().int().positive(), createdAt: z.string().datetime({ offset: true }),
  // The my page's folder view beta. Its own call saves it without a new version, so a settings page left open still saves.
  folderView: z.boolean().default(false),
})
export const profileResultSchema = z.object({ profile: profileSchema.nullable() })
export type Profile = z.infer<typeof profileSchema>
// A deleted account keeps its row under this placeholder handle; the API refuses it as a chosen handle.
export const deletedHandle = /^deleted_[0-9a-f]{11}$/
export const accountSchema = z.object({ email: z.string(), provider: z.string() })
