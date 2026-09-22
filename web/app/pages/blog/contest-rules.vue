<script setup lang="ts">
import { socialPages } from '~~/shared/social-pages'
const config = useRuntimeConfig()
const title = socialPages['/blog/contest-rules']!.title
const description = 'ShareOJのコンテストの参加方法、配点、順位、誤答ペナルティ、問題と提出の公開範囲を説明します。'
const canonical = new URL('/blog/contest-rules', config.public.siteUrl).href
useSeoMeta({ title: `${title} | ShareOJ 記事`, description, ogTitle: title, ogDescription: description, ogType: 'article', ogUrl: canonical })
useHead({ link: [{ rel: 'canonical', href: canonical }] })
useSharePreview({ title, path: '/blog/contest-rules', description })
</script>

<template>
  <article class="blog-article">
    <div class="breadcrumb-row"><nav class="breadcrumb" aria-label="パンくずリスト"><NuxtLink to="/blog">記事</NuxtLink><span aria-hidden="true">/</span><span>{{ title }}</span></nav><TweetButton :title="title" :url="canonical" /></div>
    <header class="blog-article-header">
      <p class="eyebrow">SHAREOJ GUIDE</p>
      <h1>{{ title }}</h1>
      <p class="lead">コンテストごとの開催日時、問題の配点、誤答ペナルティを確認して参加してください。</p>
    </header>
    <nav class="blog-toc" aria-label="記事の目次"><a href="#participation">参加と開催時間</a><a href="#scoring">得点と順位</a><a href="#penalty">誤答ペナルティ</a><a href="#visibility">閲覧できる内容</a><a href="#creation">コンテストの作成</a></nav>
    <section id="participation">
      <h2>参加と開催時間</h2>
      <p>コンテストの概要と順位表は、ログインせずに閲覧できます。
        ログイン後、開始前または開催中に「参加する」を押すと参加登録できます。
        登録した時点で、未提出でも0点で順位表に表示されます。</p>
      <p>開催中の提出には参加登録が必要です。
        終了後は新たに参加登録できませんが、ログインして練習提出できます。</p>
      <p>言語ごとのバージョン、コンパイル設定、追加ライブラリは、<NuxtLink to="/blog/language-guide">使える言語と実行環境の仕様</NuxtLink>を確認してください。</p>
      <p>公式順位に反映されるのは、コンテスト内の問題ページから、開始時刻以降かつ終了時刻より前に受け付けた提出です。
        終了前に受け付けた提出は、判定が終了後になっても反映されます。</p>
      <p>開始前のテスト提出、終了時刻以降の練習提出、サンプル検証は公式順位に含まれません。
        通常の問題ページからの提出も、そのコンテストの順位には反映されません。
        コンテストページの日時は日本時間で表示します。</p>
    </section>
    <section id="scoring">
      <h2>得点と順位</h2>
      <p>各問題で初めて正解（AC）すると、その問題に設定された配点を獲得します。
        部分点はなく、同じ問題に何度正解しても得点は増えません。</p>
      <p>順位は合計得点の高い順に決まります。
        同点の場合は、開始から最後の得点獲得に使われた提出を受け付けるまでの経過時間に、誤答ペナルティを加えた時間で比較します。
        この時間が短い方が上位となり、得点も時間も同じ場合は同順位です。</p>
      <p>コンテストの作成者（<strong>コンテストセッター</strong>）と、コンテスト内のいずれか1問でもテスターに登録されているユーザーは、公式順位の対象外です。</p>
      <p>順位表は画面を開いている間、15秒ごとに更新します。
        「今すぐ更新」から手動でも更新できます。</p>
    </section>
    <section id="penalty">
      <h2>誤答ペナルティ</h2>
      <p>誤答ペナルティは、正解した問題について、初回正解の提出より前に受け付けた誤答だけに加算します。
        未正解の問題の誤答や、初回正解より後の誤答は加算しません。
        判定が完了した順ではなく、提出を受け付けた順で数えます。</p>
      <ul>
        <li>加算対象：不正解（WA）、実行時エラー（RE）、時間超過（TLE）、メモリ超過（MLE）、出力超過（OLE）。</li>
        <li>対象外：コンパイルエラー（CE）、ジャッジエラー（JE）、判定待ちの提出。</li>
      </ul>
      <p>1回あたりのペナルティ時間はセッターが設定し、初期値は5分です。
        0分に設定するとペナルティは加算されません。</p>
      <p>たとえば、最後の得点獲得が開始から45分後で、正解した問題の初回正解前の誤答が合計2回、ペナルティが1回5分なら、順位の比較に使う時間は55分です。</p>
    </section>
    <section id="visibility">
      <h2>問題と提出の公開範囲</h2>
      <p>開始前の問題は、セッターと、その問題のテスターが閲覧できます。
        開始時刻になると全員が問題を閲覧でき、終了後は問題と登録済みの解説が通常の問題ページにも公開されます。
        コンテスト内の解説メニューは、終了前は作成者本人に、終了後は全員に表示します。</p>
      <p>各問題の「自分の提出」には、ログイン中のユーザーによる、その問題への提出を表示します。
        コンテスト内の問題では、そのコンテストでの提出に絞って表示します。</p>
      <p>「すべての提出」と他人の提出コードは、コンテスト終了前はセッターとテスターが閲覧できます。
        コンテスト内のいずれか1問のテスターであれば、コンテスト全体の提出を閲覧できます。
        終了後は、開始時刻以降の通常の提出と練習提出を全員が閲覧できます。</p>
      <p>開始前のテスト提出は終了後も一般公開されません。
        サンプル検証やテストケース生成の実行結果も、公開の提出一覧には掲載しません。</p>
    </section>
    <section id="creation">
      <h2>コンテストの作成と変更</h2>
      <p>セッターは自分の未公開問題を選び、出題順、各問題の配点、説明、開催期間、ペナルティ時間を設定します。
        作成したコンテストは、マイページの「コンテスト」から確認できます。</p>
      <p>問題を保存すると、開始後もコンテストの問題文・テストケース・採点設定に自動で反映されます。
        更新後に受け付けた提出から新しい内容で採点し、受付済みの提出の採点内容・結果は変更しません。
        開催期間や配点など、コンテスト自体の設定は開始後に編集できません。</p>
    </section>
    <NuxtLink class="return-link" to="/contests">コンテスト一覧へ →</NuxtLink>
  </article>
</template>
