# ADR-0003: AudioQuery は構造体にせず map[string]any で素通しする

- **Status**: Accepted (2026-07-04)

## Context

合成は VOICEVOX 互換の 2 段階 API(`POST /audio_query` → パラメータ調整 →
`POST /synthesis`)で行う。AudioQuery のフィールドは VOICEVOX ベースライン
(`speedScale`, `intonationScale`, `prePhonemeLength`, …)に加え、
AivisSpeech が独自フィールド(`tempoDynamicsScale` 等)を追加しており、
今後のエンジン更新でさらに増える可能性が高い。Go の構造体に固定すると、
未知フィールドがラウンドトリップで**黙って欠落**し、エンジン既定値との
差異として音質バグ化する(発見が非常に困難)。

## Decision

`engine.AudioQuery` は `map[string]any` とし、本サーバーが所有する
キーのみ上書きする:

- `speedScale` / `intonationScale` / `volumeScale` — 台本の演技指定
- `prePhonemeLength` / `postPhonemeLength` — config 既定値
- `outputSamplingRate` / `outputStereo` — **全行で config 値に強制統一**

それ以外のフィールドは一切触らず素通しする。

## Consequences

- エンジンの schema drift に無改修で追従できる(最大の実装リスクを吸収)。
- サンプリングレート統一により、master の concat demuxer が再エンコード
  なしで安全に連結できる(モデルごとの native rate 差異を根絶)。
- 型安全性は失うが、対象キーは 6 個に限定されておりテストで担保する
  (モックエンジンは未知フィールドを含む AudioQuery を返し、素通しを
  アサートする)。

## Alternatives considered

- **完全な構造体定義**: エンジン更新のたびに欠落バグのリスク。不採用。
- **構造体+`json.RawMessage` の残余フィールド**: Go では inline 残余の
  標準手段がなく、実装が複雑になるだけ。不採用。
