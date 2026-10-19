## 判定方法を選ぶ {#modes}

![判定方法の設定。①判定方法のタブ、②判定方法、③判定コードの言語、④判定コードの形式](/images/docs/checker.png)

作問画面の「生成・検証・判定」で①「判定方法」を開き、②で判定方法を選びます。

| 判定方法 | 使う場面 |
| --- | --- |
| 通常判定（空白区切りで比較） | 答えが1通りに決まる問題。出力と期待される出力を比べます。 |
| スペシャルジャッジ | 答えが複数ある問題や、小数の誤差を認める問題。検証コードが提出の出力を判定します。 |
| インタラクティブ（対話形式） | 提出と対話しながら進める問題。対話用ジャッジが提出とやり取りして判定します。 |

通常判定の比べ方は、[提出と判定結果](/docs/submissions#comparison)で説明しています。

スペシャルジャッジとインタラクティブでは、③で判定コードの言語を選び、コードを入力します。
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
上限を超えた場合や、コンパイルに失敗した場合は、提出全体がJEになります。

判定コードと集計コードが標準エラー出力に書いた内容は、提出結果の「ジャッジコードの診断（作問者のみ）」に表示し、問題の作者とテスターだけが見られます。
ただし、インタラクティブ問題のサンプル検証では、対話の記録と対話用ジャッジの標準エラー出力を、提出した人にも表示します。
サンプルケースで、解答の手がかりになる情報を標準エラー出力に書かないでください。
