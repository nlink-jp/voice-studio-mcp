---
name: multi-actor-narration
description: 資料・テーマ・物語を複数話者の音声へ変換する統合ワークフロー。5フォーマットを内包し、依頼内容に応じて振り分ける──対談ポッドキャスト型(ホスト＋ゲスト)、パネル解説型(司会＋専門家で論点整理・多角的議論)、ニュース/朝礼ブリーフィング型(アンカーで短時間・定期配信)、教材ナレーション型(講師＋生徒役のQ&Aで段階学習)、朗読劇/オーディオブック型(ナレーター＋登場人物で小説・脚本を朗読劇化)。PDF/Word/スライド/データ/テーマ/チャットセッション/小説原稿を入力に取り、voice-studio MCP(AivisSpeech Engine)で音声化する。「解説音声」「対談」「ポッドキャスト」「パネル」「討論」「論点整理」「ニュース」「朝礼」「ブリーフィング」「毎朝の要約音声」「教材」「講義」「レッスン」「Q&Aで学ぶ」「朗読」「ラジオドラマ」「オーディオブック」「朗読劇」「narrated audio」などの依頼で使う。voice-studio MCP サーバが必要。
argument-hint: "[資料/テーマ/原稿パス] [フォーマット任意] [workspace-root] [workspace-id]"
---

# マルチアクター音声ワークフロー集

資料・テーマ・物語を、**複数の話者による音声**へ変換する統合スキル。
資料の解説・テーマ紹介（対談 / パネル / ニュース / 教材）に加え、小説・脚本の
**朗読劇 / オーディオブック化**（audio-drama）までを1つのスキルで扱う。

音声合成は **voice-studio MCP**（AivisSpeech Engine）を使う。各フォーマットは
同一の台本 JSONL スキーマと合成パイプラインを `./_shared/` で共有する
（audio-drama のみ資料取り込みではなく物語解析を用いる）。

入力: **$ARGUMENTS**（資料パス / テーマ / 原稿パス、任意でフォーマット指定・
`workspace_root`・`workspace_id`。不明なものは聞く）。制作状態は
`<workspace_root>/<workspace_id>/` に置かれる。**書き込める絶対パスの
`workspace_root` を用意し、以降すべての voice-studio 呼び出しで同じ値を渡す**
（省略時はサーバ既定 `~/.voice-studio`）。詳細は
[_shared/PIPELINE.md](./_shared/PIPELINE.md) のワークスペース節と P0。

## ステップ0: フォーマットを選ぶ（最初に必ず）

依頼内容から下表で判断し、迷う場合や `$ARGUMENTS` で指定が無い場合は
ユーザに確認する。決めたら該当の `FORMAT.md` を読み、その固有手順に入る。

| フォーマット文書 | フォーマット | 話者構成 | 向いている用途 | 目安尺 |
|---------------|------------|---------|--------------|-------|
| [talk-podcast/FORMAT.md](./talk-podcast/FORMAT.md) | 対談ポッドキャスト型 | ホスト＋ゲスト（2〜3） | 親しみやすい解説、NotebookLM 風 | 10–20 分 |
| [panel-discussion/FORMAT.md](./panel-discussion/FORMAT.md) | パネル解説型 | 司会＋専門家（3〜4） | 論点整理、多角的な深掘り、意思決定支援 | 15–30 分 |
| [news-briefing/FORMAT.md](./news-briefing/FORMAT.md) | ニュース/朝礼ブリーフィング型 | アンカー（＋レポーター） | 要点を短時間で、定期配信 | 2–5 分 |
| [lesson-narration/FORMAT.md](./lesson-narration/FORMAT.md) | 教材ナレーション型 | 講師＋生徒役（2〜） | 段階的な学習、研修・オンボーディング | 5–10 分/単元 |
| [audio-drama/FORMAT.md](./audio-drama/FORMAT.md) | 朗読劇/オーディオブック型 | ナレーター＋登場人物 | 小説・脚本を朗読劇/オーディオブック化 | 作品次第 |

### どれを選ぶか

- 堅い資料をやわらかく伝えたい → **talk-podcast**
- 賛否・トレードオフのある論点を扱う → **panel-discussion**
- 毎朝の更新情報を手短に／自動配信したい → **news-briefing**
- 概念を順を追って教えたい → **lesson-narration**
- 小説・物語・脚本を朗読劇/オーディオブックにしたい → **audio-drama**

## 対応する入力

解説系4フォーマット（talk-podcast / panel-discussion / news-briefing /
lesson-narration）は次を入力に取れる（取り込み手順は
[_shared/SOURCE-INGEST.md](./_shared/SOURCE-INGEST.md)）:

- PDF / Word 文書（`pdf` / `docx` スキルで抽出）
- テーマ・トピック指定（資料なし。`WebSearch` で裏取り）
- スライド / データ（pptx / xlsx。`pptx` / `xlsx` スキル）
- チャットセッションそのもの（`session_info` のトランスクリプト）

audio-drama は**小説・物語・脚本の原稿**（テキスト / Markdown）を入力に取り、
要点抽出ではなく物語解析（シーン分割・話者帰属）を行う
（[audio-drama/FORMAT.md](./audio-drama/FORMAT.md)）。

## 共通基盤（_shared/）

全フォーマットが参照する共通部品。各 `FORMAT.md` は「資料→台本」の変換だけを
固有に持ち、以下は共有する。

| ファイル | 内容 |
|---------|------|
| [_shared/PIPELINE.md](./_shared/PIPELINE.md) | voice-studio 制作パイプライン（環境確認〜合成〜マスタリング〜納品）、エラー対処、禁止事項 |
| [_shared/SCRIPT-SCHEMA.md](./_shared/SCRIPT-SCHEMA.md) | 台本 JSONL スキーマと演出パラメータ（intensity/speed/pause 等） |
| [_shared/SOURCE-INGEST.md](./_shared/SOURCE-INGEST.md) | 入力タイプ別の取り込み・要点カード・事実性チェック |
| [_shared/VOICE-CATALOG.md](./_shared/VOICE-CATALOG.md) | 話者一覧スナップショットと役割別の推奨キャスティング、ライセンス早見 |

## 共通の制作フロー（要約）

1. `list_speakers` で環境と話者を確認し、`workspace_root`/`workspace_id` を用意
2. 入力資料を読み、**要点カード**へ抽出（SOURCE-INGEST）
3. フォーマット固有の**番組構成**に並べ替え（各 `FORMAT.md`）
4. **キャスティング承認**（人間チェックポイント）→ `casting.toml`
5. 発音辞書を登録
6. **台本 JSONL** を書く（SCRIPT-SCHEMA）
7. `synthesize_script` → `check_job` で合成
8. **試聴レビュー**（人間チェックポイント）→ 差分修正
9. `master` でマスタリング（mp3 配信 / m4b 章付き）
10. 納品（音声＋クレジット＋ライセンス警告）

> 2 つの人間チェックポイント（キャスティング承認・試聴レビュー）は省略しない。

> audio-drama（朗読劇）は 2 の「要点カード抽出」の代わりに物語解析（シーン分割・
> 話者帰属・地の文チャンク化）を行う。詳細は
> [audio-drama/FORMAT.md](./audio-drama/FORMAT.md)。

## ディレクトリ構成（このスキルは1セットで self-contained）

```
multi-actor-narration/            ← このディレクトリ全体で1スキル
├── SKILL.md                       ← このファイル（フォーマット選択＋共通フロー）
├── _shared/                       ← 全フォーマット共通の部品（同梱）
│   ├── PIPELINE.md
│   ├── SCRIPT-SCHEMA.md
│   ├── SOURCE-INGEST.md
│   └── VOICE-CATALOG.md
├── talk-podcast/                  ← 対談ポッドキャスト型
│   ├── FORMAT.md
│   ├── casting.template.toml
│   └── script.template.jsonl
├── panel-discussion/              ← パネル解説型
├── news-briefing/                 ← ニュース/朝礼ブリーフィング型
├── lesson-narration/              ← 教材ナレーション型
└── audio-drama/                   ← 朗読劇/オーディオブック型
```

各フォーマットは `FORMAT.md`（実行手順）＋ `casting.template.toml`（キャスト雛形）
＋ `script.template.jsonl`（台本例）で構成。すべての相互リンクはこのスキル
ディレクトリ内で閉じているため、別環境へ移しても壊れない。

## 前提

- **voice-studio MCP サーバ**が登録され、AivisSpeech Engine が起動していること。
  未起動なら `list_speakers` が `engine_unavailable` を返す。
- 音声の再生はユーザ側で行う（ツールは音声ファイルのパスを返す）。
- 商用配信時はライセンス（Non-Commercial 系・ShareAlike 系に注意）とクレジット
  同梱を必ず確認（[_shared/VOICE-CATALOG.md](./_shared/VOICE-CATALOG.md)）。
