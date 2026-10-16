// Guide pages, in reading order. The sidebar, the guide index, the footer, the sitemap and the
// share images all read this list. Each page's body is web/app/content/docs/<slug>.md.
export type DocPage = {
  slug: string
  // `title` heads the page; the shorter `label` fits the sidebar and the footer on one line.
  title: string
  label: string
  description: string
  lead: string
}

export const docSections: { title: string, pages: DocPage[] }[] = [
  {
    title: '問題を解く',
    pages: [
      { slug: 'languages', title: '使える言語と実行環境の仕様', label: '使える言語', description: 'ShareOJに提出できる言語のバージョン、コンパイル設定、追加ライブラリ、コードの書き方と実行制限を説明します。', lead: '提出するコードに合わせて、言語と実行環境を選んでください。' },
      { slug: 'difficulty', title: '難易度の目安', label: '難易度の目安', description: 'Lv.1〜10の難易度を、AtCoder Beginner ContestとAtCoder Problemsのdiffを目安に説明します。', lead: '問題選びや難易度評価に使える、Lv.1〜10の目安です。' },
    ],
  },
  {
    title: 'コンテスト',
    pages: [
      { slug: 'contest-rules', title: 'コンテストのルール', label: 'コンテストのルール', description: 'ShareOJのコンテストの参加方法、配点、順位、誤答ペナルティ、問題と提出の公開範囲を説明します。', lead: 'コンテストごとの開催日時、問題の配点、誤答ペナルティを確認して参加してください。' },
    ],
  },
  {
    title: '問題を作る',
    pages: [
      { slug: 'markdown', title: 'Markdown と数式の書き方', label: 'Markdown と数式', description: 'ShareOJ の Markdown エディターで、問題文、数式、複数行の入力形式を書く方法を例とともに紹介します。', lead: '問題の構成は自由です。Markdown で文章を組み立て、必要なところに数式を添えられます。' },
      { slug: 'generator', title: '入出力生成と入力検証の使い方', label: '入出力の生成と検証', description: 'コードによる入力と期待出力の生成、入力の制約を検証する手順をC++17の例で紹介します。', lead: 'テストケースをコードで作れます。ケース番号から入力を生成し、その入力を解答プログラムに渡して期待出力を揃えましょう。' },
    ],
  },
]

export const docPages = docSections.flatMap(section => section.pages)
export const docPath = (slug: string) => `/docs/${slug}`
export const findDoc = (slug: string) => docPages.find(page => page.slug === slug)
export const docSection = (slug: string) => docSections.find(section => section.pages.some(page => page.slug === slug))!
// Pages the site footer links directly, besides the guide index.
export const footerDocs = ['contest-rules', 'languages', 'markdown'].map(slug => findDoc(slug)!)

// Guide URLs before /docs existed. They redirect permanently so shared links and posts keep working.
export const legacyDocPaths: Record<string, string> = {
  '/blog/contest-rules': 'contest-rules',
  '/blog/language-guide': 'languages',
  '/blog/difficulty-guide': 'difficulty',
  '/blog/generator-guide': 'generator',
  '/blog/markdown-guide': 'markdown',
}
