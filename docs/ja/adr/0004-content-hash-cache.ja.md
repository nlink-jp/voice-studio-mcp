# ADR-0004: 行単位のコンテンツハッシュキャッシュ(pause は含めない)

- **Status**: Accepted (2026-07-04)

## Context

長編台本(数百行)の合成は CPU 推論で分〜十分単位かかる。制作の実態は
「台本を少し直して再実行」の反復であり、毎回全行合成すると反復コストが
制作体験を支配する。また、エージェントはジョブ喪失(サーバー再起動)から
安価に回復できる必要がある(ADR-0005)。

## Decision

行ごとに合成入力のハッシュを取り、一致すればスキップする。

- キー: `SHA256("v2|" + engineVersion|speakerUUID|styleID|speed|intensity|volume|prePhoneme|postPhoneme|samplingRate|text)`
- 保存: `cache/index.json`(line_id → {hash, duration_seconds, synthesized_at})。
  行完了ごとに temp+rename で atomic に永続化。
- ヒット条件: ハッシュ一致 **かつ** `wav/<id>.wav` が実在すること。
- `force=true` で全行バイパス(モデル更新後の作り直し等)。

**含めないもの**: `pause_after_ms`。ポーズは合成 WAV に焼かず master 時に
無音挿入で実現するため、間の調整(頻繁に起きる)で再合成が走らない。

**含めるもの**: エンジンバージョンと speaker_uuid。エンジン/モデル更新や
配役変更は出力が変わり得るため、意図的にキャッシュを無効化する。

## Consequences

- 3 行直して再実行 → 3 行だけ合成。リテイクは `wav/<id>.wav` の上書きで
  決定的。
- ジョブが消えても `synthesize_script` の再実行が実質差分実行になる
  (ADR-0005 の非永続化を成立させる前提)。
- キー構成の変更はキャッシュ全無効化を意味する(prefix `v2|` で世代管理)。

## Alternatives considered

- **mtime ベース**: 台本の再生成で全行無効化されるため不採用。
- **WAV 自体のハッシュ**: 合成しないと分からないので目的に合わない。
- **pause をキーに含める**: 間の微調整のたびに再合成が走る。不採用。
