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
[ "$local_rev" = "$remote_rev" ] && exit 0

# Actions image'ni hali yig'ayotgan bo'lsa, keyingi daqiqada qayta urinamiz
if ! docker manifest inspect "$IMAGE:$remote_rev" >/dev/null 2>&1; then
  echo "Yangi commit ${remote_rev:0:7}: image hali tayyor emas, kutilmoqda"
  exit 0
fi

echo "Yangi versiya ${remote_rev:0:7} o'rnatilmoqda"
exec ./deploy/deploy.sh
