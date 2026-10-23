#!/usr/bin/env bash
# Serverda o'yinni eng so'nggi versiyaga yangilaydi.
# GitHub Actions shu skriptni SSH orqali chaqiradi; qo'lda ham ishga tushirsa bo'ladi.
set -euo pipefail
cd "$(dirname "$(readlink -f "$0")")/.."

# autodeploy bilan bir vaqtda ishlamasin (u qulfni o'zi olib, ZARBA_LOCKED=1 bilan chaqiradi)
if [ "${ZARBA_LOCKED:-}" != 1 ]; then
  exec 9>"${TMPDIR:-/tmp}/zarba-autodeploy.lock"
  flock -w 120 9 || { echo "Boshqa yangilanish ketyapti, keyinroq urinib ko'ring" >&2; exit 1; }
fi

git fetch --quiet origin main
git merge --ff-only --quiet origin/main

# PostgreSQL paroli: .env bo'lmasa, bir marta tasodifiy parol bilan yaratiladi (git'ga tushmaydi).
# DIQQAT: .env ni o'chirmang — baza shu parol bilan yaratilgan.
if ! grep -qs '^ZARBA_DB_PASSWORD=' .env; then
  ( umask 077; echo "ZARBA_DB_PASSWORD=$(head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n')" >> .env )
  echo ".env yaratildi (PostgreSQL paroli)"
fi
docker compose pull --quiet
docker compose up -d --remove-orphans
docker image prune -f >/dev/null

# server javob berishi va baza tayyor bo'lishini kutamiz ("db":"starting" -> "ok")
out=""
for _ in $(seq 1 60); do
  if out=$(curl -fsS localhost:8080/healthz 2>/dev/null) && ! grep -q '"db":"starting"' <<<"$out"; then
    echo "Yangilandi: $(git log -1 --format='%h %s') — $out"
    exit 0
  fi
  sleep 1
done
if [ -n "$out" ]; then
  echo "Server ishlayapti, lekin baza 60 soniyada tayyor bo'lmadi: $out — docker compose logs --tail 50 db zarba" >&2
else
  echo "Server 60 soniyada javob bermadi: docker compose logs --tail 50" >&2
fi
exit 1
