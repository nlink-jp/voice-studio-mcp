#!/usr/bin/env bash
# 同梱スキルを配布用 .skill パッケージへビルドする。
# .skill は「スキルディレクトリを含む zip」。展開するとスキルディレクトリが
# 復元され、内部リンクはその中で閉じているため別環境でも壊れない。
#
# 使い方（リポジトリルートからでも可。Makefile の package-skill が呼ぶ）:
#   bash skills/build.sh                 # skills/ 内の全スキルをビルド
#   bash skills/build.sh <skill> [...]   # 指定したスキルのみビルド
#
# 生成物: dist/<skill>.skill（リポジトリルートの dist/。.gitignore 済み）
set -euo pipefail

# このスクリプトは skills/ に置かれる。skills/ を基準にスキルを走査し、
# 生成物はリポジトリルートの dist/ に出す。
SKILLS_DIR="$(cd "$(dirname "$0")" && pwd)"
DIST="$(cd "$SKILLS_DIR/.." && pwd)/dist"
cd "$SKILLS_DIR"

# スキル判定: SKILL.md があり、その先頭行が frontmatter 区切り(---) であること
is_skill() {
  local d="$1"
  [ -f "$d/SKILL.md" ] && [ "$(head -n1 "$d/SKILL.md")" = "---" ]
}

# skills/ 直下ディレクトリからスキルを自動検出
discover_skills() {
  local d
  for d in */; do
    d="${d%/}"
    is_skill "$d" && echo "$d"
  done
}

# 1スキルを検証してビルド
build_one() {
  local skill="${1%/}"
  if ! is_skill "$skill"; then
    echo "エラー: '$skill' はスキルではありません（skills/$skill/SKILL.md に frontmatter が必要）" >&2
    return 1
  fi
  # サブディレクトリに frontmatter 付き SKILL.md が無いこと（別スキル誤検出の防止）
  local nested
  nested=$(find "$skill" -mindepth 2 -name 'SKILL.md' | wc -l | tr -d ' ')
  if [ "$nested" != "0" ]; then
    echo "エラー: '$skill' のサブディレクトリに SKILL.md があります（FORMAT.md にリネームしてください）" >&2
    find "$skill" -mindepth 2 -name 'SKILL.md' >&2
    return 1
  fi
  rm -f "$DIST/$skill.skill"
  # skill/ をトップ階層に持つ zip を作り、OS のゴミは除外
  zip -rX "$DIST/$skill.skill" "$skill" \
    -x '*/.DS_Store' -x '*/__pycache__/*' -x '*.pyc' >/dev/null
  echo "ビルド完了: dist/$skill.skill"
}

# ビルド対象の決定（引数指定 or 全スキル）
skills=()
if [ "$#" -gt 0 ]; then
  skills=("$@")
else
  while IFS= read -r s; do
    [ -n "$s" ] && skills+=("$s")
  done < <(discover_skills)
fi

if [ "${#skills[@]}" -eq 0 ]; then
  echo "ビルド対象のスキルが見つかりません（frontmatter 付き SKILL.md を持つ skills/ 直下ディレクトリ）" >&2
  exit 1
fi

mkdir -p "$DIST"
built=0
for skill in "${skills[@]}"; do
  build_one "$skill"
  built=$((built + 1))
done
echo "合計 $built スキルをビルドしました: ${skills[*]}"
