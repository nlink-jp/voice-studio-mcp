# ADR-0002: エンジンライフサイクルは MCP サーバーが管理し、attach を優先する

- **Status**: Accepted (2026-07-04)

## Context

本サーバーの利用主体は自律エージェント(Claude Code / Cowork)である。
「先に人間がエンジンを起動しておく」という前提を置くと、エージェントの
ワークフローが人の準備作業でブロックされ、自律性が損なわれる。一方で
AivisSpeech の GUI アプリも同じポート(10101)でエンジンを起動するため、
無条件に spawn すると二重起動やユーザーのエンジンを勝手に殺す事故が起きる。

## Decision

`engine.mode = "managed"`(既定)では次の順で振る舞う:

1. **attach 優先**: 起動時に `GET /version` を probe し、応答があれば
   既存エンジンに接続するだけで所有権を取らない(Stop は no-op)。
2. 応答がなければ `engine.command` を子プロセスとして spawn し、
   `/version` が応答するまでポーリング(初回モデルロードは分単位なので
   `startup_timeout_seconds` は既定 180 秒)。
3. 終了時は SIGTERM → `shutdown_timeout_seconds` 待ち → SIGKILL で回収。
   spawn 直後から Wait ゴルーチンでゾンビ回収と早期終了検知を行う。

`engine.mode = "external"` は接続のみ(テスト・手動起動・リモート運用)。

## Consequences

- エージェント/ユーザーの準備作業ゼロで `serve` 一発で動く。
- GUI 管理のエンジンや、前回 SIGKILL された serve の孤児エンジンがいても
  安全(attach するだけ)。
- attach したエンジンは回収しないため、`serve` 終了後もエンジンが残る
  ケースがある(それは本サーバーが起動したものではないので正しい挙動)。
- テストは external モード+httptest モックで完全ハーメチックにできる
  (これが単体/E2E テスト戦略の要)。

## Alternatives considered

- **常に spawn**: 二重起動・GUI エンジン殺しの事故があるため不採用。
- **起動済み前提(接続のみ)**: 自律ワークフローが人に依存するため不採用。
- **launchd 等での常駐化**: 導入の複雑さに見合わない。将来検討。
