#!/usr/bin/env bash
set -euo pipefail

profile="${1:-cov.out}"

AGGREGATE_MIN=90.0
PER_PKG_MIN=75.0

INCLUDED=(
  "github.com/millsmillsymills/protonmail-mcp/cmd/protonmail-mcp"
  "github.com/millsmillsymills/protonmail-mcp/internal/server"
  "github.com/millsmillsymills/protonmail-mcp/internal/tools"
  "github.com/millsmillsymills/protonmail-mcp/internal/session"
  "github.com/millsmillsymills/protonmail-mcp/internal/protonraw"
  "github.com/millsmillsymills/protonmail-mcp/internal/proterr"
  "github.com/millsmillsymills/protonmail-mcp/internal/log"
  "github.com/millsmillsymills/protonmail-mcp/internal/keychain"
)

go tool cover -func="$profile" >"$profile.func"

pkg_pct() {
  local pkg="$1"
  awk -v p="$pkg" '
    $1 ~ p {
      split($NF, a, "%");
      total++;
      sum += a[1];
    }
    END {
      if (total == 0) print "0.0"; else printf("%.1f", sum/total);
    }
  ' "$profile.func"
}

aggregate_line=$(awk '/^total:/ {print $NF}' "$profile.func" | tr -d '%')
echo "aggregate: ${aggregate_line}%"

fail=0
for pkg in "${INCLUDED[@]}"; do
  pct=$(pkg_pct "$pkg")
  printf "  %-70s %s%%\n" "$pkg" "$pct"
  awk -v a="$pct" -v b="$PER_PKG_MIN" 'BEGIN{ exit (a+0 < b+0) ? 1 : 0 }' || { echo "FAIL: $pkg below ${PER_PKG_MIN}% floor"; fail=1; }
done

awk -v a="$aggregate_line" -v b="$AGGREGATE_MIN" 'BEGIN{ exit (a+0 < b+0) ? 1 : 0 }' || { echo "FAIL: aggregate below ${AGGREGATE_MIN}%"; fail=1; }

exit "$fail"
