# RFP: voice-studio-mcp

> Generated: 2026-07-04
> Status: Draft

## 1. Problem Statement

小説・物語の原稿からラジオドラマ/朗読音声を制作する際、脚本化・話者帰属・演技指定といった知的作業は Claude Code / Cowork エージェントが担えるが、エージェントには「声」がない。本ツールは、AivisSpeech Engine 等のローカル TTS エンジンをラップする MCP サーバーとして、話者カタログ提供(利用規約メタデータ付き)・読み辞書登録・台本一括合成・マスタリングをエージェントに提供し、完全ローカルで原稿から完成音声までの自律ワークフローを成立させる。対象ユーザーは、Claude Code / Cowork で創作ワークフローを組む個人クリエイター(まずは作者自身)。

従来のコマンドラインパイプライン(LLM 呼び出しを内蔵した自己完結型 CLI)とは異なり、判断・創作作業はすべてエージェント側に置き、本ツールは決定的な重作業(エンジン管理・合成・後処理)のみを担う。

## 2. Functional Specification

### Commands / API Surface

MCP サーバー(stdio トランスポート)。提供ツールは 6 本:

| ツール | 概要 |
|--------|------|
| `list_speakers` | 接続エンジンの話者+スタイル一覧を返す。各音声モデルの利用規約・クレジット文言をメタデータとして同梱し、エージェントがキャスティング時に規約を考慮できるようにする |
| `register_dictionary` | 作品固有の読み辞書(固有名詞・造語の読み)を一括登録。エンジンの `/user_dict_word` API を使用 |
| `synthesize_script` | ワークスペース内の台本 JSONL(ファイルパス渡し)を一括合成。行ハッシュキャッシュにより変更行のみ差分再合成。長編向けに非同期ジョブとして実行 |
| `synthesize_line` | リテイク用の単行合成 |
| `check_job` | 非同期ジョブの進捗・エラー行を構造化して返却 |
| `master` | ffmpeg による連結・ラウドネス正規化・mp3/m4b 出力。使用モデルの規約からクレジットテキストを自動生成 |

Go の慣例に従い、単一バイナリ+サブコマンド構成(`serve` ほか、デバッグ用サブコマンドを想定)。

### Input / Output

- **入力**:
  - 台本 JSONL — エンジン非依存の中間形式。1 行 = 1 発話: `{"id", "scene", "speaker", "text", "style", "intensity", "speed", "pause_after_ms"}`。スキーマの正(canonical definition)は本 MCP 側が持ち、後続のスキルはこれを参照する
  - キャスティング表 — 登場人物名 → エンジン話者 UUID + スタイル ID のマッピング(規約確認状況・クレジット文言の欄を含む)
  - 読み辞書 — 単語・読み・アクセント情報のリスト
- **出力**: 行単位 WAV 群(行 ID ベース命名でリテイク可能)、マスター音声(mp3 / m4b)、クレジットテキスト。ツール結果には音声バイトを含めず、ワークスペース内のファイルパス+行ごとの成否・尺サマリを返す(トークン節約)
- **スコープ管理**: 作品 = workspace。`workspace_id` で読み辞書・キャスティング表・合成キャッシュ・ジョブを作品単位に分離(data-toolbox-mcp と同方式)

### Configuration

- `config.toml`(エンジンバイナリパス、ポート、出力ルート、ffmpeg パス等)。lite-series の config 慣例(sectioned TOML)に従う
- `-c` フラグで複数 config 切替(ask-llm-mcp と同方式)

### External Dependencies

- **AivisSpeech Engine** — 公式配布バイナリをユーザーが導入。MCP サーバーが子プロセスとして起動・ヘルスチェック・終了回収を行う(デフォルトポート 10101、VOICEVOX 互換 HTTP API)
- **ffmpeg** — `master` ツールのみで使用。事前インストール必須(ランタイム依存)
- **Go ライブラリ依存ゼロ** — net/http + os/exec のみ。クラウド API・クレデンシャル一切なし

## 3. Design Decisions

- **言語は Go**: 単一バイナリ配布、外部ライブラリ依存ゼロの組織方針に適合。data-toolbox-mcp の骨格(workspace スコープ、構造化エラー `{code, message, details}`、子プロセス終了ステータスの表面化、内部 ID 非露出)を移植する
- **v1 は AivisSpeech Engine 専用**: API は VOICEVOX 互換のため、エンジン抽象は設計に残しつつ検証対象を 1 本に絞る。VOICEVOX ENGINE 併用(声のロスター拡大)は将来拡張
- **エンジンライフサイクルは MCP が管理**: 未起動ならspawn、終了時に回収。エージェント/ユーザーの準備作業をゼロにし、自律ワークフローを成立させる
- **macOS 専用(v1)**: エンジン spawn・モデル配置パスが OS 依存であり、主目的が macOS ローカル制作のため darwin-arm64 に集中。Windows / Linux は将来検討
- **既存ツールとの補完関係**: data-toolbox-mcp(データ分析の手)・ask-llm-mcp(相談の口)に対する「声」。後続の radio-drama スキル(skills-series、別プロジェクト)が本 MCP の消費者第一号
- **明示的スコープ外**: 脚本化・台本化・キャスティング判断(エージェント側の仕事)、BGM/SE ミックス(DAW 等で後処理)、音声モデルの学習・クローン、クラウド TTS、ストリーミング再生、skills-series 側スキル(別 RFP)

### TTS 基盤選定の根拠(2026-07 調査)

deep-research による比較調査(25 ソース・24 claim 検証済み)の結論:

- **AivisSpeech Engine** — Style-Bert-VITS2 系モデルを AIVMX(ONNX)形式で CPU 推論。macOS 13+ を Apple Silicon 推奨で公式サポート、LGPL-3.0 単独。日本語品質(アクセント・イントネーション)は SBV2 系が現行最高水準。スタイル ID + `intonationScale`(0.0〜2.0)でセリフ単位の感情表現制御が可能
- Style-Bert-VITS2 直接運用は macOS 公式サポート外+AGPL-3.0 のため不採用。kokoro 系(MIT/Apache 2.0、高速)は日本語 2 話者のみで多話者キャスティング要件を満たさず不採用。VOICEVOX はキャラクター声の拡張として将来候補

## 4. Development Plan

### Phase 1: Core

- config.toml 読み込み、エンジンライフサイクル管理(spawn / ヘルスチェック / 終了回収)
- workspace 管理(作成・列挙・削除)
- `list_speakers`、`synthesize_line`、`synthesize_script`(同期・小規模)
- 台本 JSONL スキーマ v1 + キャスティング表定義
- テスト: エンジンを httptest でモック+dummy MCP client ハーネス

### Phase 2: Features

- `synthesize_script` の非同期ジョブ化+`check_job`
- 行ハッシュキャッシュ(差分再合成)
- `register_dictionary`
- `master`(ffmpeg 連結・ラウドネス正規化・mp3/m4b)+クレジット自動生成
- 話者規約メタデータの整備

### Phase 3: Release

- 実作品(短編)での E2E 検証
- docs/{en,ja} 3 層、README.md / README.ja.md、CHANGELOG.md、AGENTS.md
- make build(darwin-arm64、Developer ID 署名+notarize)
- v0.1.0 リリース、umbrella submodule 更新、org profile 更新、check-org.sh

各 Phase は独立してレビュー可能。

## 5. Required API Scopes / Permissions

**None**。完全ローカル動作(エンジンは localhost、クラウド API・OAuth・クレデンシャル一切なし)。

## 6. Series Placement

Series: **util-series**

Reason: data-toolbox-mcp / ask-gemini-mcp / ask-llm-mcp と同じ「エージェントに能力を提供する MCP サーバー」の系譜。設計が固まっており用途も明確なため、lab-series(実験段階)ではなく util-series に配置する。開発は `_wip/voice-studio-mcp/` から開始し、統合時に umbrella へ submodule 追加する。

## 7. External Platform Constraints

- **AivisSpeech Engine の制約**:
  - 非ストリーミング(完全合成後に WAV 返却)— 本用途はバッチ生成のため許容
  - 一部 VOICEVOX API は `501 Not Implemented`(`/synthesis_morphing`、`/cancellable_synthesis` 等)
  - 必要 RAM 1.5GB+、初回起動・モデル初回ロードに待ち時間あり
  - Intel Mac は公式に積極検証外(Apple Silicon 前提)
- **音声モデルの規約**: AIVMX モデルごとに利用規約が異なる(ソフトウェアの LGPL-3.0 とは別建て)。配信物制作という性質上、キャスティング表に規約確認状況を持たせ、`master` でクレジットを自動生成することで対応
- **ffmpeg**: `master` 使用時は事前インストール必須

---

## Discussion Log

- **2026-07-04 TTS 基盤調査**: deep-research ワークフロー(25 ソース、24 claim 3-0 検証)で macOS ローカル日本語 TTS を比較。品質最優先のバッチ生成には AivisSpeech(SBV2 系・LGPL-3.0・公式 macOS 対応)、リアルタイムには kokoro 系という結論。本プロジェクトはバッチ生成用途のため AivisSpeech を採用
- **アーキテクチャ転換**: 当初は LLM 呼び出しを内蔵した Go CLI パイプライン(原稿→脚本→台本→音声)を検討したが、脚本化・台本化はエージェント(Claude Code / Cowork)自身が行う前提に変更。本ツールは音声合成部分のみを MCP サーバーとして提供する構成に転換
- **ツール名**: voice-studio-mcp / audio-drama-mcp / tts-toolbox-mcp / speech-forge-mcp を比較し、合成だけでなく辞書・キャスティング情報・マスタリングまで含む性格を表す voice-studio-mcp を採用
- **v1 エンジンスコープ**: AivisSpeech 専用に決定。VOICEVOX 互換 API のためマルチエンジン拡張余地は設計に残す
- **エンジンライフサイクル**: MCP サーバーが子プロセスとして起動・回収する方式に決定(自律ワークフローで人の準備作業を不要にするため)
- **master スコープ**: v1 に含める(ffmpeg ランタイム依存を許容)。BGM/SE ミックスは明示的にスコープ外
- **スキル分離**: 本 RFP は MCP サーバーのみ。脚本化・台本化手順を持つ radio-drama スキル(skills-series)は MCP 完成後の別プロジェクト。台本 JSONL スキーマの正は MCP 側が持つ
- **プラットフォーム**: v1 は macOS(darwin-arm64)専用。エンジン管理の OS 分岐と検証コストを抑える
- **workspace 採用**: 作品 = workspace として辞書・キャスティング・キャッシュ・ジョブを分離(data-toolbox-mcp 方式)
- **シリーズ配置**: util-series に決定(MCP サーバー系譜との整合)
