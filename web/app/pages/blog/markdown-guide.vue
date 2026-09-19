<script setup lang="ts">
import { socialPages } from '~~/shared/social-pages'
const config = useRuntimeConfig()
const canonical = new URL('/blog/markdown-guide', config.public.siteUrl).href
const title = socialPages['/blog/markdown-guide']!.title
const description = 'ShareOJ の Markdown エディターで、問題文、数式、複数行の入力形式を書く方法を例とともに紹介します。'
useSeoMeta({ title: `${title} | ShareOJ 記事`, description, ogTitle: title, ogDescription: description, ogType: 'article', ogUrl: canonical })
useHead({ link: [{ rel: 'canonical', href: canonical }] })

const basic = ['## 問題文', '', '整数 $A$ と $B$ の和を求めてください。', '', '## 制約', '', '- $0 \\le A, B \\le 10^9$', '- 入力はすべて整数です。'].join('\n')
const lineBreak = '1行目<br/>2行目'
const colors = '%赤文字ですよ%{red} と %青い文字%{#1565C0}'
const inlineMath = '整数 $A$ と $B$ の和 $A + B$ を求めてください。'
const input = ['```input', '$T$', '$\\mathrm{case}_1$', '$\\mathrm{case}_2$', '$\\vdots$', '$\\mathrm{case}_T$', '```'].join('\n')
const arrayInput = ['```input', '$N$', '$A_1 \\quad A_2 \\quad \\cdots \\quad A_N$', '```'].join('\n')
const math = ['```math', '\\sum_{i=1}^{N} i = \\frac{N(N+1)}{2}', '```'].join('\n')
const details = [':::details ヒント', 'ここに **ヒント** や数式 $A + B$ を書けます。', ':::'].join('\n')
const programLanguages = [
  ':::details 対応言語と指定する言語名',
  '| 言語 | コードブロックに指定する言語名 |',
  '| --- | --- |',
  '| Python | `py` または `python` |',
  '| C++ | `cpp` または `c++` |',
  '| C | `c` |',
  '| Rust | `rust` または `rs` |',
  '| C# | `csharp`、`cs` または `c#` |',
  '| Java | `java` |',
  '| Nim | `nim` |',
  '| Go | `go` または `golang` |',
  '| Haskell | `haskell` または `hs` |',
  '| JavaScript | `javascript` または `js` |',
  '| TypeScript | `typescript` または `ts` |',
  '| Ruby | `ruby` または `rb` |',
  '| 色分けなし | `text` または言語名を省略 |',
  ':::',
].join('\n')
const program = ['```py', 'a, b = map(int, input().split())', 'print(a + b)', '```'].join('\n')
const code = ['### 入力例 1', '', '```text', '3 5', '```', '', '### 出力例 1', '', '```text', '8', '```'].join('\n')
useSharePreview({ title, path: '/blog/markdown-guide', description })
</script>

<template>
  <article class="blog-article">
    <div class="breadcrumb-row"><nav class="breadcrumb" aria-label="パンくずリスト"><NuxtLink to="/blog">記事</NuxtLink><span aria-hidden="true">/</span><span>書き方ガイド</span></nav><TweetButton :title="title" :url="canonical" /></div>
    <header class="blog-article-header"><p class="eyebrow">SHAREOJ GUIDE</p><h1>{{ title }}</h1><p class="lead">問題の構成は自由です。Markdown で文章を組み立て、必要なところに数式を添えられます。</p></header>
    <nav class="blog-toc" aria-label="記事の目次"><a href="#structure">基本の書き方</a><a href="#inline-math">文中の数式</a><a href="#input-format">入力形式</a><a href="#display-math">独立した数式</a><a href="#samples">入出力例</a><a href="#line-breaks">改行</a><a href="#colors">文字色</a><a href="#programs">プログラム</a><a href="#details">折りたたみ</a><a href="#judge-status">ジャッジステータス</a><a href="#drafts">下書きと保存</a></nav>
    <section id="structure"><h2>見出しで問題文を組み立てる</h2><p><code>##</code> で見出し、<code>-</code> で箇条書きを書けます。太字は <code>**強調したい文字**</code>、リンクは <code>[表示する文字](URL)</code> です。表や画像も使えます。</p><p>問題文、制約、入力、出力、入出力例の順に書くと、解く人が情報を見つけやすくなります。見出しの名前や順序は自由に変えられます。</p><pre><code>{{ basic }}</code></pre></section>
    <section id="inline-math"><h2>文章の中に数式を書く</h2><p>数式を <code>$...$</code> で囲みます。変数、添字、指数も同じように書けます。</p><pre><code>{{ inlineMath }}</code></pre><div class="guide-result"><p class="guide-result-label">表示例</p><ProblemMarkdown :source="inlineMath" /></div><p>添字は <code>$A_i$</code>、指数は <code>$10^9$</code>、不等号は <code>$A \le B$</code> と書きます。ドル記号そのものを表示したいときは <code>\$</code> を使います。</p></section>
    <section id="input-format"><h2>改行を保った入力形式を書く</h2><p>コードブロックの言語名を <code>input</code> にすると、改行を保ちながら、その中の <code>$...$</code> を数式として表示します。数式の外側には通常の文字も書けます。</p><pre><code>{{ input }}</code></pre><div class="guide-result"><p class="guide-result-label">表示例</p><ProblemMarkdown :source="input" /></div><p>同じ行に変数を並べるときは、数式内の <code>\quad</code> で間隔を空けます。横方向の省略記号は <code>\cdots</code>、縦方向は <code>\vdots</code> です。</p><pre><code>{{ arrayInput }}</code></pre><div class="guide-result"><p class="guide-result-label">表示例</p><ProblemMarkdown :source="arrayInput" /></div></section>
    <section id="display-math"><h2>独立した数式を書く</h2><p><code>math</code> コードブロックには、LaTeX の数式を直接書きます。ブロック内をさらに <code>$</code> で囲む必要はありません。</p><pre><code>{{ math }}</code></pre><div class="guide-result"><p class="guide-result-label">表示例</p><ProblemMarkdown :source="math" /></div><p><code>$$...$$</code> で独立した数式を書くこともできます。複数行の数式を整列させたいときは、数式内で <code>\begin{aligned} ... \end{aligned}</code> を使えます。</p></section>
    <section id="samples"><h2>入出力例は文字列のまま書く</h2><p>具体的な入力値や出力値は <code>text</code> コードブロックに入れます。通常のコードブロックでは <code>$...$</code> も数式に変換されません。インラインコードも同様です。</p><pre><code>{{ code }}</code></pre></section>
    <section id="line-breaks">
      <h2>好きな位置で改行する</h2>
      <p>本文や表のセル内で改行したいときは <code>&lt;br/&gt;</code> を入れます。</p>
      <pre><code>{{ lineBreak }}</code></pre>
      <div class="guide-result"><p class="guide-result-label">表示例</p><ProblemMarkdown :source="lineBreak" /></div>
      <p>HTML タグは、属性のない <code>&lt;br&gt;</code>、<code>&lt;br/&gt;</code>、<code>&lt;br /&gt;</code> だけ使えます。
        コード内に書いたタグは文字のまま表示されます。</p>
    </section>
    <section id="colors">
      <h2>文字色を指定する</h2>
      <p><code>%文字%{色}</code> と書くと、囲んだ文字の色を変えられます。
        色には <code>red</code> などの CSS の色名、または <code>#1565C0</code> などの16進カラーを指定します。</p>
      <pre><code>{{ colors }}</code></pre>
      <div class="guide-result"><p class="guide-result-label">表示例</p><ProblemMarkdown :source="colors" /></div>
      <p>16進カラーは <code>#RGB</code>、<code>#RGBA</code>、<code>#RRGGBB</code>、<code>#RRGGBBAA</code> の形式に対応しています。
        たとえば <code>%白い文字%{#FFFFFF}</code> は白色になります。</p>
      <p>本文と表のセルで使えます。
        <code>%</code> で囲んだ部分は通常の文字列として扱い、その中の Markdown は変換しません。
        コードや数式の中に書いた色指定も変換されません。</p>
    </section>
    <section id="programs"><h2>プログラムを書く</h2><p>バッククォート3つに言語名を続けた <code>```py</code> などで書き始め、次の行からプログラムを書きます。最後は <code>```</code> だけの行で閉じます。改行や字下げを保ち、対応する言語ではコードの色分けと行番号が表示されます。</p><pre><code>{{ program }}</code></pre><div class="guide-result"><p class="guide-result-label">表示例</p><ProblemMarkdown :source="program" /></div><ProblemMarkdown :source="programLanguages" /></section>
    <section id="details"><h2>内容を折りたたむ</h2><p><code>:::details タイトル</code> と <code>:::</code> で本文を囲むと、タイトルをクリックして開閉できます。本文には Markdown、数式、コードブロックを使えます。問題文・記事の両方で利用できます。</p><pre><code>{{ details }}</code></pre><div class="guide-result"><p class="guide-result-label">表示例</p><ProblemMarkdown :source="details" /></div></section>
    <section id="judge-status"><h2>ジャッジステータスを表示する</h2><p><code>:AC:</code> や <code>:RE:</code> のように大文字のステータスをコロンで囲むと、バッジとして表示します。問題文・解説・記事で利用できます。コードや数式の中では変換されません。</p><pre><code>:AC: :WA: :TLE: :MLE: :OLE: :RE: :CE: :JE: :WJ:</code></pre><div class="guide-result"><p class="guide-result-label">表示例</p><ProblemMarkdown source=":AC: :WA: :TLE: :MLE: :OLE: :RE: :CE: :JE: :WJ:" /></div></section>
    <section id="drafts"><h2>問題を保存する</h2><p>ログインして編集した問題は自動保存されます。「保存」ボタンでも保存できます。</p><p>保存した問題は「自分の問題」から一覧で確認し、別の端末でも編集を再開できます。「新しい問題を作成」から別の問題を作成できます。</p><p>保存しても、問題は公開されません。</p></section>
    <NuxtLink class="return-link" to="/problems/new">エディターへ戻る →</NuxtLink>
  </article>
</template>
