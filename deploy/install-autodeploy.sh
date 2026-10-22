#!/usr/bin/env bash
# Avtomatik yangilanishni yoqadi: systemd taymeri har daqiqada deploy/autodeploy.sh ni chaqiradi.
# Ishlatish (loyiha papkasida, loyiha egasi bo'lgan foydalanuvchi nomidan): ./deploy/install-autodeploy.sh
set -euo pipefail
DIR=$(cd "$(dirname "$(readlink -f "$0")")/.." && pwd)
RUN_AS=${SUDO_USER:-$(id -un)}

if [ "$RUN_AS" != root ] && ! id -nG "$RUN_AS" | tr ' ' '\n' | grep -qx docker; then
  echo "$RUN_AS docker guruhida emas. Avval: sudo usermod -aG docker $RUN_AS (keyin qayta kiring)" >&2
  exit 1
fi
chmod +x "$DIR/deploy/deploy.sh" "$DIR/deploy/autodeploy.sh"

sudo tee /etc/systemd/system/zarba-autodeploy.service >/dev/null <<EOF
[Unit]
Description=Zarba Nuqtasi: GitHub'dagi yangi versiyani o'rnatish
After=docker.service network-online.target
Wants=network-online.target

[Service]
Type=oneshot
User=$RUN_AS
WorkingDirectory=$DIR
ExecStart=$DIR/deploy/autodeploy.sh
EOF

sudo tee /etc/systemd/system/zarba-autodeploy.timer >/dev/null <<EOF
[Unit]
Description=Zarba Nuqtasi: har daqiqada yangilanishni tekshirish

[Timer]
OnBootSec=1min
OnUnitActiveSec=1min
AccuracySec=10s

[Install]
WantedBy=timers.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now zarba-autodeploy.timer
echo "Yoqildi. Holat: systemctl list-timers zarba-autodeploy.timer"
echo "Jurnal:  journalctl -u zarba-autodeploy -f"
