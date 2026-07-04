# ADR-0001: v1 の TTS バックエンドを AivisSpeech Engine 専用とする

- **Status**: Accepted (2026-07-04)

## Context

本プロジェクトの品質要件は「流暢な日本語(アクセント・イントネーション)」
「複数話者」「完全ローカル(macOS / Apple Silicon)」「商用パッケージ製品を
除外したライセンス」である。2026-07 の比較調査(deep-research、25ソース・
24claim検証)で以下が確認された:

- **Style-Bert-VITS2 系**が日本語品質の現行最高水準だが、リポジトリ直接
  利用は macOS が公式サポート外(確認環境は Windows/WSL2/Linux のみ、
  MPS では音声破損報告)かつコードが AGPL-3.0。
- **AivisSpeech Engine** は SBV2 系モデルを AIVMX(ONNX)形式で CPU 推論
  し、macOS 13+ を Apple Silicon 推奨で公式サポート。ライセンスは
  VOICEVOX のデュアルライセンスから LGPL-3.0 のみを単独継承。API は
  VOICEVOX 互換 HTTP。
- **kokoro 系**(MIT/Apache 2.0)は高速だが日本語プリセット話者が2種のみで
  多話者キャスティング要件を満たさない。
- **VOICEVOX** はキャラクター声が豊富だが、品質面で SBV2 系に対する優位が
  なく、Mac 版は CPU モードのみ。
- **fish-speech 等**はモデルが非商用系ライセンスで除外。

## Decision

v1 は AivisSpeech Engine 専用とする。ただし API は VOICEVOX 互換なので、
`internal/engine` のクライアントはエンジン固有名を持たない汎用実装とし、
将来 VOICEVOX ENGINE(声のロスター拡大)を追加接続できる余地を残す。

## Consequences

- 日本語品質・話者拡張(AivisHub / SBV2 学習モデルの AIVM 変換)・macOS
  公式対応・LGPL を一挙に満たす。
- エンジンは非ストリーミング(完全合成後に WAV 返却)だが、本プロジェクトは
  バッチ生成用途なので許容(リアルタイム読み上げはスコープ外)。
- AivisSpeech のインストールがランタイム前提になる(バイナリ同梱はしない。
  1GB 超であり、モデル管理は AivisSpeech 側の責務)。

## Alternatives considered

- **SBV2 直接運用**: macOS 非公式+AGPL のため不採用。同等品質を LGPL で
  得られる AivisSpeech 経由が実務的。
- **kokoro (mlx-audio / kokoro-onnx)**: リアルタイム性は最良だが日本語
  2話者で要件未達。
- **マルチエンジン同時対応を v1 から**: 規約メタデータ整備と検証対象が
  倍になるため Phase 2 以降に先送り。
