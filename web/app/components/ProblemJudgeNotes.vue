<script setup lang="ts">
import type { PublicProblem } from '~~/shared/types/problem'
import { epsilonExponent, errorJudgeModeLabel } from '~/utils/error-judge'
// What a solver needs to know about the judging, under the statement of a problem and of a contest's problem. brief
// keeps the contest page's shorter wording for interactive and special-judge problems.
defineProps<{ problem: Pick<PublicProblem, 'interactive' | 'runTwice' | 'errorJudge' | 'specialJudge'>, brief?: boolean }>()
</script>
<template>
  <p v-if="problem.interactive" class="muted">{{ brief ? 'インタラクティブ問題：応答を待つ前に出力をflushしてください。' : 'インタラクティブ問題：標準入出力でジャッジと対話します。応答を待つ前に出力をflushしてください。' }}</p>
  <p v-if="problem.runTwice" class="muted">2ステップ形式（Run-twice）：プログラムは1ケースにつき2回実行されます。応答を待つ前に出力をflushしてください。</p>
  <p v-if="problem.errorJudge" class="muted">誤差ジャッジ：{{ errorJudgeModeLabel(problem.errorJudge.mode) }}が10<sup>{{ epsilonExponent(problem.errorJudge.epsilon) }}</sup>以下なら正解です。</p>
  <p v-if="problem.specialJudge" class="muted">{{ brief ? 'スペシャルジャッジ問題です。' : 'スペシャルジャッジ問題：提出の出力を検証コードで判定します。' }}</p>
</template>
