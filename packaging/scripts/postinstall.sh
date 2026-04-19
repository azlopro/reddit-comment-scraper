#!/bin/bash
set -e

SERVICE_USER="reddit-monitor"
NOLOGIN=$(command -v nologin 2>/dev/null || echo /usr/sbin/nologin)

if ! id "${SERVICE_USER}" &>/dev/null; then
  useradd \
    --system \
    --no-create-home \
    --shell "${NOLOGIN}" \
    --comment "reddit-monitor daemon" \
    "${SERVICE_USER}"
fi

chown root:root /opt/reddit-monitor/reddit-monitor
chmod 755      /opt/reddit-monitor/reddit-monitor
chmod 600      /etc/reddit-monitor/env

systemctl daemon-reload
if systemctl is-active --quiet reddit-monitor; then
  systemctl restart reddit-monitor
else
  systemctl enable --now reddit-monitor
fi
