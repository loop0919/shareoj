## 判定方法を選ぶ {#modes}

![判定方法の設定。①判定方法のタブ、②判定方法、③判定コードの言語、④判定コードの形式](/images/docs/checker.png)

作問画面の「生成・検証・判定」で①「判定方法」を開き、②で判定方法を選びます。

| 判定方法 | 使う場面 |
| --- | --- |
| 通常判定（空白区切りで比較） | 答えが1通りに決まる問題。出力と期待される出力を比べます。 |
| スペシャルジャッジ | 答えが複数ある問題や、誤差ジャッジにない条件で比べる問題。検証コードが提出の出力を判定します。 |
| インタラクティブ（対話形式） | 提出と対話しながら進める問題。対話用ジャッジが提出とやり取りして判定します。 |
| 2ステップ形式（Run-twice） | 提出を1ケースにつき2回実行し、1回目の結果を2回目へ渡す問題。2ステップ用ジャッジが2回とも提出とやり取りして判定します。 |
| 誤差ジャッジ | 答えが実数の問題。期待される出力との誤差を、選んだ条件で比べます。 |

通常判定の比べ方は、[提出と判定結果](/docs/submissions#comparison)で説明しています。
誤差ジャッジの設定と比べ方は、[誤差ジャッジ](#error-judge)で説明しています。

スペシャルジャッジ、インタラクティブ、2ステップ形式では、③で判定コードの言語を選び、コードを入力します。
言語は、解答の言語と同じでなくてもかまいません。
④で「testlib形式（Codeforces互換）」を選べるのは、言語がC++23（GCC）とC++23（Clang）のときだけです。
判定コードは公開しません。

## 現行形式の検証コード {#checker}

検証コードは、テストケースごとに提出の出力を受け取り、終了コードで結果を返します。

- 標準入力：提出の出力
- 第1引数：テストケースの入力ファイル
- 第2引数：期待される出力のファイル（空でもかまいません）
- 第3引数：提出のソースコードのファイル
- 第4引数：スコアのファイル（「[集計コードで採点する](#aggregator)」をオンにした問題でだけ読みます）

終了コードが0なら正解、それ以外なら不正解です。
異常終了した場合も不正解になり、標準出力の内容は使いません。

次の例は、提出の出力と期待される出力をそれぞれ小数として読み、絶対誤差または相対誤差が $10^{-6}$ 以下なら正解とします。
値のあとに余分な出力があれば不正解です。

```cpp
#include <algorithm>
#include <cmath>
#include <fstream>
#include <iostream>
#include <string>

int main(int argc, char* argv[]) {
    std::ifstream answer(argv[2]);
    double expected, actual;
    answer >> expected;
    if (!(std::cin >> actual)) return 1;
    std::string extra;
    if (std::cin >> extra) return 1;
    return std::abs(actual - expected) <= 1e-6 * std::max(1.0, std::abs(expected)) ? 0 : 1;
}
```

検証コードは、提出が正常に終了したケースだけで実行します。

## testlib形式 {#testlib}

testlib形式では`#include "testlib.h"`を使えます。
`inf`はテスト入力、`ouf`は提出の出力、`ans`は正解ファイルを読みます。
次は、整数を1つずつ読んで比べるcheckerです。

```cpp
#include "testlib.h"
int main(int argc, char** argv) {
  registerTestlibCmd(argc, argv);
  int actual = ouf.readInt();
  int expected = ans.readInt();
  if (actual != expected) quitf(_wa, "different answer");
  quitf(_ok, "accepted");
}
```

対話形式では`registerInteraction`で初期化します。
次の例は、入力ファイルの整数を提出へ送り、その2倍が返ることを確かめます。

```cpp
#include "testlib.h"
#include <iostream>
int main(int argc, char** argv) {
  registerInteraction(argc, argv);
  int n = inf.readInt();
  std::cout << n << std::endl;
  int actual = ouf.readInt();
  if (actual != 2 * n) quitf(_wa, "expected twice n");
  quitf(_ok, "accepted");
}
```

`_ok`はAC、`_wa`、`_pe`、`_dirt`、`_unexpected_eof`はWA、`_fail`はJEです。
`quitp`は「[集計コードで採点する](#aggregator)」をオンにした問題でだけ使え、`_pc`には対応していません。
現行形式と違い、判定コードが異常終了した場合はJEになります。
`tout`の内容を別のcheckerで判定する処理はないので、interactor自身で正誤を決めてください。

## インタラクティブ問題 {#interactive}

提出と対話用ジャッジを同時に実行し、互いの標準入出力をつなぎます。
テストケースの入力は提出へ自動では送らないので、対話用ジャッジが入力ファイルを読み、必要な情報を提出へ出力してください。
現行形式の対話用ジャッジは、検証コードと同じ4つの引数を受け取ります。
提出も対話用ジャッジも、相手の応答を待つ前に出力をフラッシュしてください。

- 提出がTLE、MLE、OLE、REになった場合は、その結果になります。
- 対話用ジャッジが正常に終了し、提出も終了すると正解です。現行形式では、対話用ジャッジが0以外の終了コードで終了すると不正解です。testlib形式では、[testlib形式](#testlib)と同じ規則で扱います。
- 対話全体の経過時間の上限は、実行時間制限に5秒を足した長さの3倍に1秒を足した長さです。

Rubyで解くときの注意は、[使える言語と実行環境の仕様](/docs/languages#libraries)に書いています。

## 2ステップ形式 {#run-twice}

提出を1ケースにつき2回実行し、1回目の結果を2回目へ渡します。
符号化と復元のように、2回目が1回目の入力を知らずに解く問題に使います。
作問者は、2回とも提出とやり取りする**2ステップ用ジャッジ**を1つ書きます。
書き方は[対話用ジャッジ](#interactive)と同じで、受け取る引数と入力が回によって次のように変わります。

| | 1回目 | 2回目 |
| --- | --- | --- |
| 現行形式の第5引数 | `1` | `2` |
| 現行形式の第6引数（引き継ぎファイル） | 空のファイル。書き込めます | 1回目に書いた内容。読み取り専用です |
| testlib形式の`inf` | テストケースの入力 | 1回目に`tout`へ書いた内容 |

現行形式の第1〜第4引数と、testlib形式の`ans`（期待される出力）は、2回とも同じです。

- 提出の2回の実行は別々の環境で行い、ファイルもメモリも持ち越せません。
- システムは何回目かを提出へ伝えません。ジャッジが出力の先頭などで伝え、その方法を問題文にも書いてください。
- testlib形式では、何回目かが引数で渡らず、2回目は元の入力を読めません。2回目に要る情報は、1回目に`tout`へ書いてください。
- 引き継ぐ内容は16 MiBまでです。超えた場合や、通常のファイルでない場合は、提出全体がJEになります。
- 時間とメモリの制限は1回ごとです。ケースのCPU時間と最大メモリは2回の大きいほう、経過時間は2回の合計です。
- 1回目で不正解や制限超過になると、2回目は実行しません。正解には、2回とも提出が正常に終了することが必要です。
- [診断](#limits)は「1回目:」と「2回目:」の見出しで分かれます。2回目を実行しなかったケースには、1回目の判定を書きます。
- 点（[集計コード](#aggregator)）は2回目に書きます。1回目の点は使いません。testlib形式の1回目の`quitp`は、「集計コードで採点する」をオンにした問題でだけ正解として扱い、それ以外の問題ではJEです。
- 1ケースが最長で2回分の時間を使います。ケース数が多いと、採点全体の時間の上限（1,800秒）を超えてJEになりやすくなります。部分点の問題では、1,500秒を過ぎると残りのケースを実行しません。

次の問題を例にします。

- 1回目：ジャッジは`1`と、長さ $N$ の二進文字列 $S$ を1行ずつ送ります。提出は、0と1からなる長さ1以上 $3N$ 以下の文字列 $T$ を1行で返します。
- 2回目：ジャッジは`2`と、$T$ のどこかに0か1を1文字挿入した文字列を送ります。提出は $S$ を返します。

テストケースの入力は、1行目が $S$、2行目が挿入の位置を決める0以上の整数 $p$ と文字 $c$ で、期待される出力は使いません。
ジャッジは、$T$ の先頭から $k$ 文字の後ろ（$k$ は $p$ を $T$ の長さ+1で割った余り）に $c$ を挿入します。
現行形式の2ステップ用ジャッジは次のとおりです。

```python
import sys

with open(sys.argv[1]) as file:
    s = file.readline().strip()
    p, c = file.readline().split()
run, handoff = sys.argv[5], sys.argv[6]
if run == '1':
    print(1, s, sep='\n', flush=True)
    t = sys.stdin.readline().strip()
    assert 1 <= len(t) <= 3 * len(s) and set(t) <= {'0', '1'}
    k = int(p) % (len(t) + 1)
    with open(handoff, 'w') as file:
        file.write(t[:k] + c + t[k:])
else:
    with open(handoff) as file:
        print(2, file.read(), sep='\n', flush=True)
    assert sys.stdin.readline().strip() == s
```

testlib形式の例では、テストケースの入力の先頭に`1`の行を足し、1回目は`tout`の先頭に`2`を書きます。
2回目は元の入力を読めないので、$S$ も`tout`に書きます。

```cpp
#include "testlib.h"
#include <iostream>
#include <string>

int main(int argc, char** argv) {
  registerInteraction(argc, argv);
  int run = inf.readInt();
  std::string s = inf.readToken();
  std::cout << run << '\n';
  if (run == 1) {
    int p = inf.readInt();
    std::string c = inf.readToken();
    std::cout << s << std::endl;
    std::string t = ouf.readToken();
    if (t.size() > 3 * s.size() || t.find_first_not_of("01") != std::string::npos) quitf(_wa, "invalid code");
    t.insert(p % (t.size() + 1), c);
    tout << 2 << '\n' << s << '\n' << t << std::endl;
    quitf(_ok, "encoded");
  }
  std::string t = inf.readToken();
  std::cout << t << std::endl;
  if (ouf.readToken() != s) quitf(_wa, "wrong string");
  quitf(_ok, "decoded");
}
```

次の提出は、どちらの形式のジャッジでも正解になります。

- 1回目は、$S$ の各文字を3回ずつ並べて $T$ とします。
- 2回目は、同じ文字が続く区間ごとに、長さを3で割って四捨五入した数だけ文字を戻します。1文字が挿入されていても、これで $S$ に戻ります。

```python
from itertools import groupby

if input() == '1':
    print(''.join(bit * 3 for bit in input()), flush=True)
else:
    print(''.join(bit * round(len(list(group)) / 3) for bit, group in groupby(input())), flush=True)
```

## 誤差ジャッジ {#error-judge}

答えが実数の問題を、判定コードを書かずに採点します。
②で「誤差ジャッジ」を選び、「種類」「許容誤差」「判定の余裕」を設定します。
許容誤差 $\varepsilon$ は $10^{-1}$〜$10^{-12}$ の10のべき乗から選び、初期値は $10^{-6}$ です。

出力と期待される出力を空白と改行で区切り、同じ位置の語（トークン）を1組ずつ比べます。
文字列として同じなら一致、両方が数なら次の条件で比べ、それ以外は不一致です。

| 種類 | 正解になる条件 |
| --- | --- |
| 絶対誤差 | $\lvert a - b \rvert \le T$ |
| 相対誤差 | $\lvert a - b \rvert \le T \times \lvert b \rvert$ |
| 絶対誤差または相対誤差（初期値） | 上のどちらか |

$a$ は提出の値、$b$ は期待される出力の値、$m$ は判定の余裕（%）で、$T = \varepsilon \times (1 + m/100)$ です。
誤差は10進のまま正確に計算し、$T$ ちょうどなら正解です。
種類が相対誤差で $b$ が0なら、提出も0のときだけ正解です。

| | 例 |
| --- | --- |
| 数として読む | `3`、`-0.5`、`2.`、`.25`、`1e-3`、`6.02E+23` |
| 数として読まない | `+1.0`、`0x10`、`1_000`、`nan`、`inf`、400バイトを超えるトークン、指数の絶対値が400を超えるトークン（`1e401`） |

数は値で比べるので、`1e3` と `1000`、`-0` と `0` は同じです。

「判定の余裕」は、期待される出力と真の値の差 $\delta$ を見込んで、$\varepsilon$ を広げる割合です。
絶対誤差の場合、真の値との誤差が $\varepsilon$ 以内の解答は、期待される出力と最大 $\varepsilon + \delta$ 離れます。
$\delta \le \varepsilon \times m/100$ なら、この解答も正解になります。

| 判定の余裕 | 吸収できる $\delta$ |
| --- | --- |
| 0.1%（初期値） | $\varepsilon$ の1,000分の1まで |
| 1% | $\varepsilon$ の100分の1まで |
| 0% | なし（期待される出力が正確な場合だけ） |

- 期待される出力は十分な桁数で出してください。
  $\varepsilon$ が $10^{-6}$ なら、小数第10位以上（相対誤差だけで比べる場合は有効数字10桁以上）が目安です。
- トークンの数が違うと、数を比べる前に不正解になります。
- 数でないトークンは完全一致で比べます。
  `Yes` と `yes` は別の出力です。
- 空白と改行は区別せず、行の数や1行に並べる数も確かめません。
- 期待される出力に数が数百万個あると、1ケースの比較に数十秒かかることがあります。
  この時間も採点全体の時間の上限（1,800秒）に含まれます。

## 集計コード {#aggregator}

[部分点採点](/docs/authoring#scoring)で「集計コードで採点する」をオンにした問題では、判定コードがケースごとに整数の点を返せます。
現行形式では、ACかWAのケースでスコアのファイルに書いた0〜$10^{18}$の整数を読みます。
空なら点なしで、それ以外の内容は提出全体がJEになります。
testlib形式では、`quitp`に0〜1,000,000の整数を渡すと、ACとしてその点を付けます。
小数の`quitp`と`_pc`はJEです。

集計コードは、すべてのケースのあとに1回だけ実行し、グループごとの得点を決めます。

- 標準入力：1行目にケース数 $N$ とグループ数 $G$、次の $G$ 行に各グループの配点、次の $N$ 行に各ケースの「判定 点 サンプル 所属の数 グループ番号…」。判定はAC、WA、TLE、MLE、OLE、RE、SKIPPED（未実行）のどれかで、点がなければ`-`、サンプルは1か0、グループ番号は0から数えます。
- 標準出力：各グループの得点を1行ずつ、0以上その配点以下の10進の小数で書きます。小数第3位以下は切り捨てます。

次の例は、各グループの得点を「配点×属するケースの点（0〜100）の最小÷100」にします。
点のないケースは、ACなら100、それ以外なら0とみなします。

```cpp
#include <algorithm>
#include <cstdio>
#include <iostream>
#include <string>
#include <vector>

int main() {
    int n, g;
    std::cin >> n >> g;
    std::vector<long long> points(g), ratio(g, 100);
    for (auto& p : points) std::cin >> p;
    for (int i = 0; i < n; i++) {
        std::string verdict, score;
        int sample, k;
        std::cin >> verdict >> score >> sample >> k;
        long long value = score != "-" ? std::min(std::stoll(score), 100LL) : verdict == "AC" ? 100 : 0;
        for (int j = 0; j < k; j++) {
            int group;
            std::cin >> group;
            ratio[group] = std::min(ratio[group], value);
        }
    }
    for (int j = 0; j < g; j++) {
        long long hundredths = points[j] * ratio[j];  // 100倍した得点
        std::printf("%lld.%02lld\n", hundredths / 100, hundredths % 100);
    }
}
```

集計コードの上限は判定コードと同じで、失敗や書式違いは提出全体がJEになります。

## 判定コードの上限と診断 {#limits}

判定コードは、各ケースでCPU時間5秒、メモリは検証コードが512 MiB、対話用ジャッジが256 MiBまでです。
2ステップ用ジャッジは、対話用ジャッジと同じ上限を1回ごとに使います。
上限を超えた場合や、コンパイルに失敗した場合は、提出全体がJEになります。

判定コードと集計コードが標準エラー出力に書いた内容は、提出結果の「ジャッジコードの診断（作問者のみ）」に表示し、問題の作者とテスターだけが見られます。
ただし、インタラクティブ問題と2ステップ形式のサンプル検証では、対話の記録とジャッジの標準エラー出力を、提出した人にも表示します。
サンプルケースで、解答の手がかりになる情報を標準エラー出力に書かないでください。
