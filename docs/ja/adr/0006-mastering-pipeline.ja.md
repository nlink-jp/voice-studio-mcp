# ADR-0006: マスタリングは concat demuxer + 1パス loudnorm、音量は2段構え

- **Status**: Accepted (2026-07-04)

## Context

完成音声は「行 WAV を台本順に、行ごとの間(pause_after_ms)を挟んで連結し、
聴感音量を揃え、配布形式にエンコード」して得る。設計上の論点は
(1) ffmpeg での連結方法、(2) ラウドネス正規化の方式と目標値、
(3)「音量」という語が指す2つの異なる要件の分離、(4) チャプター埋め込み。

## Decision

1. **連結は concat demuxer + 無音 WAV ファイル方式**。
   distinct な pause 値ごとに無音 WAV を1つだけ生成(dedupe)し、
   `concat.txt` に行 WAV と無音を交互に列挙して一発変換する。
   フィルタグラフ(`aevalsrc`/`adelay` を行数分連結)はコマンド長が台本長に
   比例して爆発するため採らない。
2. **ラウドネス正規化は 1 パス loudnorm、目標 -18 LUFS(既定)**。
   -16(ポッドキャスト標準)で開始したが実聴で「大きい」とのフィードバックを
   受け、約0.8倍に相当する -18(オーディオブックのレンジ)に変更。
   `master.loudnorm_i` で調整可能。2 パス(measure→apply)は精度が上がるが
   v1 では見送り(将来改善)。
3. **音量は2段構え**: 行単位 `volume`(エンジン volumeScale、キャッシュ
   キーに含む)は行間の**相対バランス**(ささやき/叫び)用。最終出力の
   **聴感音量**は loudnorm が支配するため `loudnorm_i` で制御する。
   この分離を README/スキーマ説明に明記する(volumeScale を上げても
   loudnorm に打ち消される、という混乱を防ぐ)。
4. **m4b は ipod muxer + ffmetadata チャプター**。チャプター開始時刻は
   行 WAV の実測尺+pause の累積から事前計算する(loudnorm は尺を変えない
   ため正確)。プレイヤー互換性リスクに備え `chapters=false` で無効化可能、
   mp3 を第一級出力とする。

## Consequences

- 長編でも ffmpeg 呼び出しは「無音生成(distinct pause 数)+最終変換 1 回」
  で済む。
- 未合成行があると `master_incomplete`(missing_line_ids 付き)で拒否し、
  部分的なマスターを黙って作らない。
- 1 パス loudnorm は 2 パス比で正確性が落ちるが、朗読・ドラマ用途の
  実用上は許容(実測: 目標 -18 に対し -18.2 LUFS)。

## Alternatives considered

- **フィルタグラフ一発合成**: コマンド長爆発+デバッグ困難。不採用。
- **合成時に音量を焼き込んで正規化を省略**: 行間の音量差が残り、モデル間の
  収録レベル差も吸収できない。不採用。
- **既定 -16 LUFS 維持**: 実聴フィードバックにより -18 へ変更。
