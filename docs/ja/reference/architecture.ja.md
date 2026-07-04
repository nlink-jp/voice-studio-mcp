# voice-studio-mcp アーキテクチャ

> 対象バージョン: v0.1.0 系。設計判断の「なぜ」は各 ADR
> ([../adr/](../adr/))が正であり、本書は全体像と判断の索引を提供する。

## 0. 一言でいうと

エージェント(Claude Code / Cowork)が書いた台本 JSONL を、ローカルの
AivisSpeech Engine で音声化し、ffmpeg でラジオドラマ/朗読音声に仕上げる
MCP stdio サーバー。**判断・創作はエージェント側、決定的な重作業は本サーバー
側**という責務分割が全設計の出発点である。

```
┌────────────────────┐  stdio (JSON-RPC / MCP)
│ エージェント        │◄────────────────────────┐
│ (Claude Code等)    │                          │
│  ・脚本化/台本化    │   ┌──────────────────────┴──┐
│  ・キャスティング判断│   │ voice-studio-mcp (Go)    │
│  ・演技指定         │   │  tools ── synth ── engine │──► AivisSpeech Engine
│  ・規約確認(人間と) │   │    │       │        client│    (HTTP 127.0.0.1:10101,
└────────────────────┘   │    │     cache             │     子プロセス管理)
        │ ファイルで受け渡し │  job    workspace        │
        ▼                 │  master ────────────────►│──► ffmpeg
  <workspace>/script/     └──────────────────────────┘
  <workspace>/casting.toml
```

## 1. プロセス境界と信頼モデル

| 境界 | 内容 |
|------|------|
| stdio | MCP クライアントとの JSON-RPC。**stdout は転送路専用**、ログは stderr / ファイルのみ |
| localhost HTTP | AivisSpeech Engine(managed 時は本サーバーの子プロセス)。ネットワーク外部への通信は一切ない |
| 子プロセス | エンジン(ADR-0002)と ffmpeg(Runner interface 経由)。終了コード・stderr 末尾は構造化エラーで表面化 |
| ファイルシステム | workspace ルート配下のみ書き込む。エージェント指定の相対パスは `ResolveInside` で封じ込め(絶対パス・`..` 拒否) |

シークレットは存在しない(クラウド API・認証なし)。脅威モデルの中心は
「エージェントの誤った引数によるワークスペース外への書き込み/削除」であり、
workspace_id の正規表現検証+パス封じ込め+削除時の直下チェックで防ぐ。

## 2. パッケージ構成と依存方向

```
cmd (cobra: serve/doctor/version)
 └─ tools ──► workspace, engine, script, synth, job, master, toolerr, config
              synth ──► engine, script, workspace
              job   ──► toolerr のみ(Run クロージャで synth を注入)
              master──► script, synth(WAV解析), workspace
mcpserver ──► jsonrpc, transport, toolerr   (プロトコル層はドメインを知らない)
```

- 外部 Go 依存は `cobra` と `BurntSushi/toml` のみ。エンジンと ffmpeg は
  **ランタイム依存**(コード依存にしない)。
- 骨格(transport/jsonrpc/mcpserver/toolerr/logging/e2e ハーネス)は
  data-toolbox-mcp から移植した組織共通パターン。

## 3. データフロー

### 3.1 synthesize_script(中核)

```
台本JSONL ─ Parse(全行走査・エラー収集) ─ casting.Validate(全参照解決)
    │  どちらかで失敗 → invalid_script / casting_unresolved(先頭20件のdetails付き)
    ▼
行ごとに: cacheキー計算(ADR-0004) ─ hit → skip
    │ miss
    ▼
POST /audio_query → 所有キーのみ上書き(ADR-0003) → POST /synthesis
    ▼
wav/<id>.wav (temp+rename) + cache/index.json 更新(行ごとatomic)
```

enqueue 前バリデーションを同期で完走させるのが要点: エージェントは
「投げたら 30 分後に半分失敗」ではなく「投げる前に全問題を一括で知る」。
ジョブ実行は server-lifetime context + 共有セマフォ(ADR-0005)。

### 3.2 master

```
全行のwav存在+フォーマット検証(欠落→master_incomplete)
 → distinct pause値ごとに無音WAV生成 → concat.txt(行順に交互列挙)
 → (m4bのみ) scene境界からffmetadataチャプター生成(尺+pauseの累積で事前計算)
 → ffmpeg 一発変換: concat → loudnorm(-18 LUFS既定) → mp3/m4b
 → casting から credits.txt 生成 + unverified_models 警告(ADR-0007)
```

### 3.3 コントラクト(エージェントとの境界)

- **台本 JSONL スキーマ**(`internal/script.Line`)が canonical。後続の
  radio-drama スキル(別プロジェクト)はこれを参照する。
- 大きな入力(台本)は**ファイルパスで渡す**(ツール引数に本文を載せない)。
  出力も**パス+サマリのみ**返し、音声バイトは決して返さない。トークン
  経済のための一貫した設計。

## 4. ワークスペース(作品=workspace)

```
~/.voice-studio/<id>/
├── script/           台本JSONL(エージェントが作成)
├── casting.toml      配役+規約確認記録(ADR-0007)
├── dict/words.json   辞書登録の記録(冪等性の根拠)
├── wav/<line_id>.wav 行単位出力(リテイクは同ID上書き)
├── cache/index.json  合成キャッシュ(ADR-0004)
└── master/           完成音声+credits+tmp(concat.txt等、デバッグ用に残す)
```

辞書・キャッシュ・ジョブを作品単位に分離することで、複数作品の並行制作と
「作品ごと消す」(delete)が安全になる。

## 5. エラーモデル

すべてのツールエラーは `{code, message, details}` の構造化 JSON
(isError=true の text ブロック)で返す。方針:

- **code はエージェントが分岐するための安定スラグ**(invalid_script,
  casting_unresolved, engine_unavailable, master_incomplete, …)。
- **details は修正に必要な機械可読情報を全部載せる**(不正行の一覧、
  未解決話者、欠落 line_id、ffmpeg の exit code+stderr 末尾)。
  ただし一覧系は先頭 20 件+truncated フラグで有界化。
- **message には回復手順を書く**(例: job_not_found は「再実行すれば
  キャッシュで差分だけ走る」)。利用者が自律エージェントなので、エラー
  メッセージの文言そのものが回復ループの品質を決める。

## 6. テスト戦略

| 層 | 手段 | ポイント |
|----|------|---------|
| ユニット | `go test ./...` | AivisSpeech / ffmpeg 不要で全パス(組織ルール)。エンジンは `enginetest` の httptest モック(未知フィールド応答・障害注入つき)、ffmpeg は Runner interface の fake |
| E2E(モック) | `make test-e2e` | ビルド済みバイナリを stdio で駆動する dummy MCP client ハーネス。external モード+モックエンジン+ffmpeg スタブでフルフロー |
| E2E(実機) | `VOICE_STUDIO_TEST_REAL_ENGINE=1` | opt-in。実エンジン spawn→合成→回収(real_engine_test.go)と、実データから動的キャスティングして mp3/m4b まで作るフル制作シミュレーション(real_engine_full_test.go) |

「external モード」(ADR-0002)がモック接続のテストシームを兼ねているのが
構造上のポイント。

## 7. 主要設計判断の索引

| ADR | 判断 |
|-----|------|
| [0001](../adr/0001-aivisspeech-engine-backend.ja.md) | v1 は AivisSpeech Engine 専用(SBV2 品質を LGPL+macOS 公式対応で得る) |
| [0002](../adr/0002-managed-engine-lifecycle.ja.md) | エンジンは managed 子プロセス、attach 優先 |
| [0003](../adr/0003-audioquery-passthrough.ja.md) | AudioQuery は map で素通し、サンプリングレートのみ強制統一 |
| [0004](../adr/0004-content-hash-cache.ja.md) | コンテンツハッシュキャッシュ(pause 除外・engineVersion 含む) |
| [0005](../adr/0005-in-memory-jobs.ja.md) | ジョブ非永続(回復はキャッシュ前提の再実行) |
| [0006](../adr/0006-mastering-pipeline.ja.md) | concat demuxer+1パス loudnorm(-18 LUFS)、音量2段構え |
| [0007](../adr/0007-license-metadata-as-data.ja.md) | モデル規約はデータとして人手記録、ツールが運ぶ |
| [0008](../adr/0008-license-collection-from-manifests.ja.md) | 規約の収集は AIVM マニフェストから自動化(declared)、確認は人間(verified) |

## 8. スコープ外(v1 で意図的にやらないこと)

脚本化・台本化・キャスティング判断(エージェントの仕事)/BGM・SE ミックス
(DAW 等の後処理)/音声モデルの学習・クローン/クラウド TTS/ストリーミング
再生/Windows・Linux(エンジン管理が OS 依存のため darwin-arm64 に集中)。
