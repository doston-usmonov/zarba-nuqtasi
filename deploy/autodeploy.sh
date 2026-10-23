#!/usr/bin/env bash
# systemd taymeri har daqiqada chaqiradi: GitHub'da yangi commit bo'lsa va Actions uning
# image'ini yig'ib bo'lgan bo'lsa, deploy.sh ni ishga tushiradi. Aks holda jim chiqadi.
set -euo pipefail
cd "$(dirname "$(readlink -f "$0")")/.."

exec 9>"${TMPDIR:-/tmp}/zarba-autodeploy.lock"
flock -n 9 || exit 0

IMAGE="${IMAGE:-ghcr.io/doston-usmonov/zarba-nuqtasi}"

git fetch --quiet origin main
local_rev=$(git rev-parse HEAD)
remote_rev=$(git rev-parse origin/main)
if [ "$local_rev" = "$remote_rev" ]; then
  # kod yangi, lekin .env da yangi kalit yo'q (eski deploy.sh bilan yangilangan) — bir marta qayta o'rnatamiz
  if ! grep -qs '^ZARBA_TURN_SECRET=' .env; then
    echo ".env da ZARBA_TURN_SECRET yo'q: deploy.sh qayta ishga tushirilmoqda"
    ZARBA_LOCKED=1 exec ./deploy/deploy.sh
  fi
  exit 0
fi

# Actions image'ni hali yig'ayotgan bo'lsa, keyingi daqiqada qayta urinamiz
if ! docker manifest inspect "$IMAGE:$remote_rev" >/dev/null 2>&1; then
  echo "Yangi commit ${remote_rev:0:7}: image hali tayyor emas, kutilmoqda"
  exit 0
fi

echo "Yangi versiya ${remote_rev:0:7} o'rnatilmoqda"
ZARBA_LOCKED=1 exec ./deploy/deploy.sh
