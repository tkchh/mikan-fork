package subs

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// FreeTurnNode contains server-side parameters of a node running FreeTurn.
type FreeTurnNode struct {
	Name       string
	Peer       string // "host:port"
	ObfProfile string // "rtpopus3"
	ObfKey     string
	WgPubKey   string
}

// FreeTurnClient contains user-specific credentials for FreeTurn and WireGuard.
type FreeTurnClient struct {
	ClientID   string
	WgPrivKey  string
	WgIP       string // "10.13.13.2/32"
	ClientName string
}

// BuildFreeTurnWireGuardConfig formats a clean WireGuard client config for the FreeTurn Android app.
func BuildFreeTurnWireGuardConfig(node FreeTurnNode, client FreeTurnClient) string {
	var sb strings.Builder
	sb.WriteString("[Interface]\n")
	sb.WriteString("PrivateKey = " + client.WgPrivKey + "\n")
	sb.WriteString("Address = " + client.WgIP + "\n")
	sb.WriteString("DNS = 1.1.1.1, 77.88.8.8\n")
	sb.WriteString("MTU = 1280\n")
	sb.WriteString("\n[Peer]\n")
	sb.WriteString("PublicKey = " + node.WgPubKey + "\n")
	sb.WriteString("AllowedIPs = 0.0.0.0/0, ::/0\n")
	sb.WriteString("Endpoint = 127.0.0.1:9000\n")
	sb.WriteString("PersistentKeepalive = 25\n")
	return sb.String()
}

// BuildFreeTurnLink creates a single freeturn:// share URL embedding both server and WireGuard configs.
func BuildFreeTurnLink(node FreeTurnNode, client FreeTurnClient) string {
	wgConf := BuildFreeTurnWireGuardConfig(node, client)

	wire := map[string]any{
		"v":         1,
		"provider":  "vk",
		"peer":      node.Peer,
		"transport": "udp",
		"name":      node.Name,
		"wg":        wgConf,
	}

	if node.ObfProfile != "" && node.ObfProfile != "none" {
		wire["obf"] = node.ObfProfile
		wire["key"] = node.ObfKey
	}
	if client.ClientID != "" {
		wire["cid"] = client.ClientID
	}

	raw, err := json.Marshal(wire)
	if err != nil {
		return ""
	}
	return "freeturn://" + base64.RawURLEncoding.EncodeToString(raw)
}

// BuildFreeTurnSub generates a multi-node subscription text in the sub.md format recognized by free-turn-proxy.
func BuildFreeTurnSub(title string, usedBytes, totalBytes int64, nodes []FreeTurnNode, client FreeTurnClient) string {
	var sb strings.Builder
	sb.WriteString("# name: " + title + "\n")
	if usedBytes >= 0 {
		sb.WriteString(fmt.Sprintf("# used: %.2f GB\n", float64(usedBytes)/(1024*1024*1024)))
	}
	if totalBytes > 0 {
		sb.WriteString(fmt.Sprintf("# available: %.2f GB\n", float64(totalBytes)/(1024*1024*1024)))
	}

	for _, n := range nodes {
		link := BuildFreeTurnLink(n, client)
		if link != "" {
			sb.WriteString(link + "\n")
			sb.WriteString("## name: " + n.Name + "\n")
		}
	}
	return sb.String()
}

// GenerateWireGuardKeypair produces a standard Curve25519 keypair encoded in Base64 for WireGuard.
func GenerateWireGuardKeypair() (privB64, pubB64 string, err error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	privB64 = base64.StdEncoding.EncodeToString(priv.Bytes())
	pubB64 = base64.StdEncoding.EncodeToString(priv.PublicKey().Bytes())
	return privB64, pubB64, nil
}

// GenerateClientID generates a cryptographically random 32-char hex token for free-turn-proxy.
func GenerateClientID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// NextFreeIP finds the first unallocated IPv4 address in the given subnet (e.g. 10.13.13.0/24).
func NextFreeIP(usedIPs []string, networkCIDR string) (string, error) {
	used := make(map[string]bool)
	for _, ip := range usedIPs {
		clean := strings.TrimSuffix(ip, "/32")
		used[clean] = true
	}

	base := "10.13.13"
	if idx := strings.LastIndex(networkCIDR, "."); idx > 0 {
		base = networkCIDR[:idx]
	}

	for i := 2; i <= 254; i++ {
		candidate := fmt.Sprintf("%s.%d", base, i)
		if !used[candidate] {
			return candidate + "/32", nil
		}
	}
	return "", fmt.Errorf("no free IP addresses left in subnet %s", networkCIDR)
}

