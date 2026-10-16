import { docPages, docPath } from './docs'

export const socialPages: Record<string, { title: string, kind: string }> = {
  '/': { title: 'Share Online Judge', kind: 'ShareOJ' },
  '/problems': { title: '問題', kind: 'PROBLEMS' },
  '/contests': { title: 'コンテスト', kind: 'CONTESTS' },
  '/blog': { title: '記事', kind: 'ARTICLES' },
  '/docs': { title: 'ガイド', kind: 'GUIDE' },
  ...Object.fromEntries(docPages.map(page => [docPath(page.slug), { title: page.title, kind: 'GUIDE' }])),
  '/terms': { title: '利用規約', kind: 'TERMS' },
  '/privacy': { title: 'プライバシーポリシー', kind: 'PRIVACY' },
}
