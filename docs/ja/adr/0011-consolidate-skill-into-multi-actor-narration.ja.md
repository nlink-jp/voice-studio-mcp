# ADR-0011: 同梱スキルを multi-actor-narration に一本化し、.skill を別アセットで配布する

- **Status**: Accepted (2026-07-05)
- **Amends**: ADR-0009（同梱スキルの正体と配布方法を改定。「in-repo で整合を担保する」原則は覆さず強化する）

## Context

ADR-0009 は、エージェント向けの制作スキル(radio-drama)を本リポジトリに
同梱し、サーバーの契約と歩調を合わせて進化させる決定だった。その後、
2点の変化があった:

- radio-drama スキルは **novel-works 固有の結合**を持っていた——エージェントに
  「novel-works-style の 設定資料/キャラクター早見表」を優先せよと指示していた。
  novel-works はクローズドで非公開のプロジェクトであり、汎用公開 MCP が
  これを前提にするのは不適切。
- リポジトリ外(`magifd2/claude-skills`)で、より高機能なスキル
  **multi-actor-narration** が成熟した。同一の voice-studio スキーマ上で
  4つの解説フォーマット(talk-podcast / panel-discussion / news-briefing /
  lesson-narration)を束ねる self-contained なルーターで、`.skill`
  パッケージング規約(スキルディレクトリをトップ階層に持つ zip)も備える。
  ADR-0009 自身が「必要になれば zip 同梱は後から足せる」と含みを残していた。

radio-drama の 物語→朗読劇 能力そのものは汎用的で残す価値がある——
不適切だったのは novel-works 前提の部分だけ。

## Decision

1. **同梱スキルを `multi-actor-narration` 単一に一本化する。** 旧 radio-drama
   スキルは削除し、その 物語→朗読劇/オーディオブック 能力は
   multi-actor-narration 内の**汎用化した `audio-drama` フォーマット**として
   復活させる。novel-works 結合は「キャラクター参照資料(設定資料・
   キャラクター早見表・一人称/口調/呼称の一覧など形式は問わない)が
   あれば最初に読む」という形式非依存の指示に置き換える。
2. **スキル source は in-repo を継続(ADR-0009 を強化)。**
   `skills/skills_test.go` はマルチファイル構成のスキル全体(SKILL.md
   ルーター + `_shared/*.md` + `<format>/FORMAT.md`)を集約し、全ツール・
   全エラーコード・全スキーマフィールドの掲載と、frontmatter 付き `SKILL.md`
   がちょうど1つであること(ネストした manifest が壊す不変条件)を検査する。
3. **スキルを別 `.skill` アセットとしてパッケージ・リリースする。**
   `skills/build.sh`(`skills/` 配下のスキルを自動検出し、ネスト SKILL.md
   禁止の不変条件を強制)＋ `make package-skill` が
   `dist/multi-actor-narration.skill` を生成。MCP バイナリと同じ GitHub
   release に、ただし**別アセット**として添付する——ライフサイクルが異なる
   (notarize 不要のプレーンなスキル zip)。
4. **novel-works は専用スキルを private に保持する。** 原稿固有の radio-drama
   結合は公開ツールのスコープ外に置く。

## Consequences

- 公開ツールに novel-works 前提が残らず、1スキルで 朗読劇 ＋ 解説4種を
  カバーする。
- スキルはサーバースキーマにピン留めされ続ける——ADR-0009 の整合保証が
  マルチファイル構成でも成立する。
- **破壊的変更**: `/radio-drama` コマンド名は消え、依頼は
  `/multi-actor-narration` にルーティングされる(description が
  朗読/ラジオドラマ/オーディオブック/novel/manuscript のトリガーを持つ)。
  `/radio-drama` を呼んでいた novel-works は移行するか private コピーを
  保持する必要がある——本リポジトリ外のフォローアップとして記録。
- スキル配布は2経路になる: リポジトリ保持者向けの `make install-skill` と、
  それ以外の全員向けの `.skill` リリースアセット(ADR-0009 が残した
  「zip 同梱は後から」を実現)。

## Alternatives considered

- **スキルを skills-series / 独立リポジトリへ移す**: in-repo の coherence
  test が壊れる(リポジトリ間のスキーマ同期＝ドリフト)。ADR-0009 と同じ
  理由で不採用。
- **radio-drama と multi-actor-narration を併存させる**: 合成パイプラインが
  重複し `_shared` の内容も二重化する。物語ユースケースは1フォーマットとして
  きれいに収まる。ルーター一本に統合する方を採用。
- **.skill を MCP バイナリ zip に同梱する**: 別種のアーティファクトと
  ライフサイクルを結合してしまう。別アセットの方が clean で、スキルだけ
  欲しい利用者がスキルのみ取得できる。
