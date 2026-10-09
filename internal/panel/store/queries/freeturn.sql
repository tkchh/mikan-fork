-- name: GetNodeFreeTurn :one
SELECT * FROM node_freeturn WHERE node_id = $1;

-- name: ListNodeFreeTurn :many
SELECT * FROM node_freeturn WHERE enabled = 1;

-- name: ListAllNodeFreeTurn :many
SELECT * FROM node_freeturn ORDER BY node_id;

-- name: SaveNodeFreeTurn :exec
INSERT INTO node_freeturn (node_id, enabled, listen_port, obf_profile, obf_key, wg_port, wg_network, wg_pubkey, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (node_id) DO UPDATE SET
  enabled = excluded.enabled,
  listen_port = excluded.listen_port,
  obf_profile = excluded.obf_profile,
  obf_key = excluded.obf_key,
  wg_port = excluded.wg_port,
  wg_network = excluded.wg_network,
  wg_pubkey = excluded.wg_pubkey,
  updated_at = excluded.updated_at;

-- name: DeleteNodeFreeTurn :exec
DELETE FROM node_freeturn WHERE node_id = $1;

-- name: GetUserFreeTurn :many
SELECT * FROM user_freeturn WHERE user_id = $1 ORDER BY id;

-- name: GetUserFreeTurnByClientID :one
SELECT * FROM user_freeturn WHERE client_id = $1;

-- name: ListFreeTurnPeersForNode :many
SELECT
  f.id,
  f.user_id,
  u.name AS user_name,
  s.name AS slot_name,
  f.client_id,
  f.wg_pubkey,
  f.wg_ip,
  f.device_name,
  (u.status = 'active') AS allowed
FROM user_freeturn f
JOIN users u ON u.id = f.user_id
JOIN slots s ON s.id = f.slot_id
ORDER BY f.id;

-- name: ListUsedFreeTurnIPs :many
SELECT wg_ip FROM user_freeturn;

-- name: CreateUserFreeTurn :one
INSERT INTO user_freeturn (user_id, slot_id, client_id, wg_pubkey, wg_privkey, wg_ip, device_name, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: DeleteUserFreeTurn :exec
DELETE FROM user_freeturn WHERE id = $1;

-- name: DeleteUserFreeTurnOfUser :exec
DELETE FROM user_freeturn WHERE user_id = $1;
