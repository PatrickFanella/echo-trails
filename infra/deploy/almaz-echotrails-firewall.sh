#!/usr/bin/env bash
set -euo pipefail

HOST_IP="${HOST_IP:-10.0.0.200}"
NUC_IP="${NUC_IP:-10.0.0.56}"
PORT="${ECHOTRAILS_MODERATION_PORT:-18081}"
CHAIN="ALMAZ-ECHOTRAILS-GUARD"

iptables -w -N "$CHAIN" 2>/dev/null || true
iptables -w -F "$CHAIN"
iptables -w -A "$CHAIN" -m conntrack --ctstate RELATED,ESTABLISHED -j RETURN
iptables -w -A "$CHAIN" -s "$NUC_IP/32" -p tcp -m conntrack \
  --ctorigdst "$HOST_IP" --ctorigdstport "$PORT" --ctdir ORIGINAL -j RETURN
iptables -w -A "$CHAIN" -p tcp -m conntrack \
  --ctorigdst "$HOST_IP" --ctorigdstport "$PORT" --ctdir ORIGINAL -j DROP
iptables -w -A "$CHAIN" -j RETURN

while iptables -w -C DOCKER-USER -j "$CHAIN" 2>/dev/null; do
  iptables -w -D DOCKER-USER -j "$CHAIN"
done
iptables -w -I DOCKER-USER 1 -j "$CHAIN"
