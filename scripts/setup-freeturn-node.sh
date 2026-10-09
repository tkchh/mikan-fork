#!/usr/bin/env bash
# ==============================================================================
# Mikan Panel - FreeTurn (vk-turn-proxy) Node Setup Script
# ==============================================================================
set -euo pipefail

WG_IFACE="${WG_IFACE:-ft-wg0}"
WG_PORT="${WG_PORT:-51820}"
WG_SUBNET="${WG_SUBNET:-10.14.14.0/24}"
WG_SERVER_IP="${WG_SERVER_IP:-10.14.14.1}"
FREETURN_PORT="${FREETURN_PORT:-56392}"
OBF_PROFILE="${OBF_PROFILE:-rtpopus3}"
OBF_KEY="${OBF_KEY:-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef}"
AUTH_DIR="/opt/free-turn-proxy/auth"

echo "=== FreeTurn Node Sidecar Setup ==="
if [ "$(id -u)" -ne 0 ]; then
  echo "Error: run as root" >&2
  exit 1
fi

DEFAULT_IFACE=$(ip -4 route show default | awk "{print \$5}" | head -n1)
if [ -z "$DEFAULT_IFACE" ]; then DEFAULT_IFACE="eth0"; fi

echo "--> Installing packages..."
if command -v apt-get >/dev/null 2>&1; then
  apt-get update -y && apt-get install -y wireguard wireguard-tools iptables openssl curl
elif command -v apk >/dev/null 2>&1; then
  apk add --no-cache wireguard-tools iptables openssl curl
fi

echo "--> Configuring sysctl..."
sysctl -w net.ipv4.ip_forward=1 >/dev/null || true

WG_DIR="/etc/wireguard"
mkdir -p "$WG_DIR"
chmod 700 "$WG_DIR"

if [ ! -f "$WG_DIR/${WG_IFACE}.key" ]; then
  wg genkey | tee "$WG_DIR/${WG_IFACE}.key" | wg pubkey > "$WG_DIR/${WG_IFACE}.pub"
  chmod 600 "$WG_DIR/${WG_IFACE}.key"
fi

WG_PRIVKEY=$(cat "$WG_DIR/${WG_IFACE}.key")
WG_PUBKEY=$(cat "$WG_DIR/${WG_IFACE}.pub")

cat <<EOF > "$WG_DIR/${WG_IFACE}.conf"
[Interface]
Address = ${WG_SERVER_IP}/24
ListenPort = ${WG_PORT}
PrivateKey = ${WG_PRIVKEY}
MTU = 1280

PostUp = iptables -A FORWARD -i ${WG_IFACE} -j ACCEPT; iptables -A FORWARD -o ${WG_IFACE} -j ACCEPT; iptables -t nat -A POSTROUTING -s ${WG_SUBNET} -o ${DEFAULT_IFACE} -j MASQUERADE
PostDown = iptables -D FORWARD -i ${WG_IFACE} -j ACCEPT; iptables -D FORWARD -o ${WG_IFACE} -j ACCEPT; iptables -t nat -D POSTROUTING -s ${WG_SUBNET} -o ${DEFAULT_IFACE} -j MASQUERADE
EOF
chmod 600 "$WG_DIR/${WG_IFACE}.conf"

systemctl enable "wg-quick@${WG_IFACE}" || true
systemctl restart "wg-quick@${WG_IFACE}" || true

mkdir -p "$AUTH_DIR"
if [ ! -f "$AUTH_DIR/clients.json" ]; then
  echo "{\"clients\": {}}" > "$AUTH_DIR/clients.json"
fi
chown -R 65532:65532 "/opt/free-turn-proxy" 2>/dev/null || true
chmod -R 775 "/opt/free-turn-proxy"

echo "=== FreeTurn Node Sidecar Ready ==="
echo "WireGuard Public Key: $WG_PUBKEY"
echo "WireGuard Port: $WG_PORT"
echo "WireGuard Subnet: $WG_SUBNET"
echo "FreeTurn Listen Port: $FREETURN_PORT"
echo "Obfuscation Profile: $OBF_PROFILE"
echo "Obfuscation Key: $OBF_KEY"
