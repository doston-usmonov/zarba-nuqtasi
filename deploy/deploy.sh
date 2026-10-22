#!/usr/bin/env bash
# Serverda o'yinni eng so'nggi versiyaga yangilaydi.
# GitHub Actions shu skriptni SSH orqali chaqiradi; qo'lda ham ishga tushirsa bo'ladi.
set -euo pipefail
cd "$(dirname "$(readlink -f "$0")")/.."

git pull --ff-only --quiet
docker compose pull --quiet
docker compose up -d --remove-orphans
docker image prune -f >/dev/null

for _ in $(seq 1 20); do
  if out=$(curl -fsS localhost:8080/healthz 2>/dev/null); then
    echo "Yangilandi: $(git log -1 --format='%h %s') — $out"
    exit 0
  fi
  sleep 1
done
echo "Server 20 soniyada javob bermadi: docker compose logs --tail 50" >&2
exit 1
