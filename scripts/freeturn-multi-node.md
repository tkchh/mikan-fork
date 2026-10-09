# FreeTurn Multi-Node Setup Guide

This guide explains how to deploy FreeTurn (VK TURN proxy and obfuscated WireGuard) on additional / remote Mikan nodes.

## Overview

1. **free-turn-proxy**: A TURN server proxy listening on an obfuscated UDP port (default: `56392/udp`). Obfuscates client UDP traffic using profiles like `rtpopus3`.
2. **WireGuard kernel interface (`ft-wg0`)**: Listens locally on port `51820/udp`, terminating client WireGuard sessions with MTU 1280.
3. **mikan-node**: Runs on the node, dynamically syncing `clients.json` and managing WireGuard peers on `ft-wg0` based on users active subscription status and quota.

## Automated Setup on Remote Node

Run the automated setup script on the remote node:

```bash
sudo ./scripts/setup-freeturn-node.sh
```

Environment variables for custom settings:
- `WG_IFACE`: Interface name (default: `ft-wg0`)
- `WG_PORT`: WireGuard UDP port (default: `51820`)
- `WG_SUBNET`: Subnet for this node (default: `10.14.14.0/24`)
- `FREETURN_PORT`: Public FreeTurn listen port (default: `56392`)
- `OBF_PROFILE`: Obfuscation profile (default: `rtpopus3`)
- `OBF_KEY`: Obfuscation hex key (auto-generated if omitted)

## Docker Compose Configuration on Remote Node

Ensure `mikan-node` in `compose.yaml` has `cap_add: [NET_ADMIN]` and mounts the auth directory:

```yaml
services:
  node:
    image: ${MIKAN_IMAGE}
    entrypoint: ["/usr/local/bin/mikan-node"]
    network_mode: host
    cap_add: [NET_BIND_SERVICE, NET_ADMIN]
    environment:
      MIKAN_DATA_DIR: /data/node
      MIKAN_NODE_SOCKET: /run/mikan/node.sock
      FREETURN_CLIENTS_FILE: /opt/free-turn-proxy/auth/clients.json
      FREETURN_WG_IFACE: ft-wg0
    volumes:
      - ./data/node:/data/node
      - run:/run/mikan
      - /opt/free-turn-proxy/auth:/opt/free-turn-proxy/auth:rw
```

## Registering FreeTurn in Mikan Panel

Once the node is running, configure FreeTurn in the Mikan panel via API:

```bash
curl -X PATCH https://<MIKAN_PANEL_DOMAIN>/api/v1/nodes/<NODE_ID>/freeturn \
  -H "Authorization: Bearer <ADMIN_API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "listen_port": 56392,
    "obf_profile": "rtpopus3",
    "obf_key": "<OBF_KEY>",
    "wg_port": 51820,
    "wg_network": "10.14.14.0/24",
    "wg_pubkey": "<NODE_WG_PUBKEY>"
  }'
```

The panel will automatically push the FreeTurn desired state and authorized peers to the remote node.
