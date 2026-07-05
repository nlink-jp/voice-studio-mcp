# multi-actor-narration

資料・テーマ・物語を、**複数の話者による音声**へ変換する Claude 用スキル。
1つのスキルで5つの番組フォーマットを内包し、依頼内容に応じて振り分ける。
音声合成は [voice-studio MCP](#前提)（AivisSpeech Engine）を利用する。

資料の解説・テーマ紹介（対談 / パネル / ニュース / 教材）に加え、小説・脚本の
**朗読劇 / オーディオブック化**（audio-drama）までを1つのスキルで扱う。

## 5つのフォーマット

| フォーマット | 話者構成 | 向いている用途 | 目安尺 |
|------------|---------|--------------|-------|
| [対談ポッドキャスト型](./talk-podcast/FORMAT.md) | ホスト＋ゲスト（2〜3） | 親しみやすい解説、NotebookLM 風 | 10–20 分 |
| [パネル解説型](./panel-discussion/FORMAT.md) | 司会＋専門家（3〜4） | 論点整理、多角的な深掘り、意思決定支援 | 15–30 分 |
| [ニュース/朝礼ブリーフィング型](./news-briefing/FORMAT.md) | アンカー（＋レポーター） | 要点を短時間で、定期配信 | 2–5 分 |
| [教材ナレーション型](./lesson-narration/FORMAT.md) | 講師＋生徒役（2〜） | 段階的な学習、研修・オンボーディング | 5–10 分/単元 |
| [朗読劇/オーディオブック型](./audio-drama/FORMAT.md) | ナレーター＋登場人物 | 小説・脚本を朗読劇/オーディオブック化 | 作品次第 |

解説系4フォーマットは PDF / Word / スライド / データ / テーマ指定 / チャット
セッションを、audio-drama は小説・脚本の原稿を入力に取る。

## 使い方

導入後は `/multi-actor-narration` の1コマンドに集約され、依頼内容（または冒頭の
確認）でフォーマットを振り分ける。実行手順・フォーマット選択・共通フローの正本は
[SKILL.md](./SKILL.md)。

## 構成

```
multi-actor-narration/          ← これ全体で1スキル（self-contained）
├── SKILL.md                     ← スキル本体（frontmatter・フォーマット選択・共通フロー）
├── _shared/                     ← 全フォーマット共通の部品
│   └── PIPELINE.md / SCRIPT-SCHEMA.md / SOURCE-INGEST.md / VOICE-CATALOG.md
├── talk-podcast/     { FORMAT.md, casting.template.toml, script.template.jsonl }
├── panel-discussion/ { FORMAT.md, casting.template.toml, script.template.jsonl }
├── news-briefing/    { FORMAT.md, casting.template.toml, script.template.jsonl }
├── lesson-narration/ { FORMAT.md, casting.template.toml, script.template.jsonl }
└── audio-drama/      { FORMAT.md, casting.template.toml, script.template.jsonl }
```

frontmatter を持つのは `SKILL.md` の1つだけ。各フォーマットは `FORMAT.md`（別スキル
として誤検出されないよう `SKILL.md` という名前は使わない）＋雛形ファイルで構成。
すべての相互リンクはこのディレクトリ内で閉じているため、ディレクトリごと移動しても
壊れない。共通基盤は [_shared/](./_shared/)（PIPELINE / SCRIPT-SCHEMA / SOURCE-INGEST
/ VOICE-CATALOG）。

## 前提

- **voice-studio MCP サーバ**が登録され、AivisSpeech Engine が起動していること。
  未起動の場合 `list_speakers` が `engine_unavailable` を返す。
- 音声の再生はユーザ側で行う（ツールは音声ファイルのパスを返す）。

## ライセンスに関する注意

音声話者ごとにライセンスが異なる。**商用配信時は Non-Commercial 系や ShareAlike
義務のある話者を除外**し、クレジット同梱を必ず確認すること。詳細は
[_shared/VOICE-CATALOG.md](./_shared/VOICE-CATALOG.md)。
