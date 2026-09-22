# ShareOJ 紹介スライド

初めてShareOJを知るエンジニアに向けた、約15分の紹介資料です。
本編16枚と参照資料1枚で、利用体験、全体構成、採点処理、隔離、認証、運用を説明します。

- [編集可能なPowerPoint](shareoj-introduction.pptx)
- [配布用PDF](shareoj-introduction.pdf)
- [発表者ノートと参照ファイル](speaker-notes.md)

内容の基準は2026年9月22日、コミット`c59a2e1`です。
本番環境の状態を今回再検証した資料ではありません。
実画面のスクリーンショットを使わず、編集可能な図形で操作と構成を示しています。
PowerPointの各ページにも発表者ノートを含めています。

## 再生成

Python 3とIPAexGothicを用意してください。
このスクリプトはLinuxの`/usr/share/fonts/opentype/ipaexfont-gothic/ipaexg.ttf`を使用します。
PowerPointでもIPAexGothicをインストールすると改行と配置を維持できます。
PDFには使用フォントを埋め込んでいます。

```sh
python3 -m venv /tmp/shareoj-slides-venv
/tmp/shareoj-slides-venv/bin/pip install python-pptx==1.0.2 reportlab==5.0.1
/tmp/shareoj-slides-venv/bin/python docs/presentations/build.py
```

`build.py`からPowerPointとPDFを同じ座標で直接生成します。
テキストは画像化していないため、PowerPointで編集でき、PDFでも検索できます。
生成時に参照ファイルの存在、ページ数、テキスト幅とスライド内への収まりを検証します。
