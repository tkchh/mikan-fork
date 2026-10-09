package subs

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildFreeTurnLinkAndSub(t *testing.T) {
	node := FreeTurnNode{
		Name:       "Home Server",
		Peer:       "95.191.18.145:56392",
		ObfProfile: "rtpopus3",
		ObfKey:     "9e76eda0cd8952c0f73000465d3e62846615ddf01becc46686113b781daa3cf8",
		WgPubKey:   "kd/usf3zuT6/HYeKroPZPXRhcMRa5dIcKvL1OoIexV4=",
	}

	client := FreeTurnClient{
		ClientID:   "16de5a24014937f200a5d7d87b1a1617",
		WgPrivKey:  "+LZDmYJy1Gor+kGH9bo2mY0CcTUZTowJpxEYwxROulM=",
		WgIP:       "10.13.13.2/32",
		ClientName: "Test User",
	}

	link := BuildFreeTurnLink(node, client)
	if !strings.HasPrefix(link, "freeturn://") {
		t.Fatalf("expected link to start with freeturn://, got %s", link)
	}

	payload := strings.TrimPrefix(link, "freeturn://")
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("failed to decode base64url: %v", err)
	}

	var wire map[string]any
	if err := json.Unmarshal(decoded, &wire); err != nil {
		t.Fatalf("failed to parse json wire: %v", err)
	}

	if wire["provider"] != "vk" {
		t.Errorf("expected provider vk, got %v", wire["provider"])
	}
	if wire["peer"] != "95.191.18.145:56392" {
		t.Errorf("expected peer 95.191.18.145:56392, got %v", wire["peer"])
	}
	if wire["cid"] != client.ClientID {
		t.Errorf("expected cid %s, got %v", client.ClientID, wire["cid"])
	}

	wgConf, ok := wire["wg"].(string)
	if !ok || !strings.Contains(wgConf, "[Interface]") || !strings.Contains(wgConf, "Endpoint = 127.0.0.1:9000") {
		t.Errorf("wire[wg] does not contain expected WireGuard configuration: %v", wire["wg"])
	}

	sub := BuildFreeTurnSub("Mikan VIP", 1024*1024*1024, 10*1024*1024*1024, []FreeTurnNode{node}, client)
	if !strings.Contains(sub, "# name: Mikan VIP") {
		t.Errorf("expected subscription title, got %s", sub)
	}
	if !strings.Contains(sub, "freeturn://") {
		t.Errorf("expected subscription to include link, got %s", sub)
	}
}

func TestFreeTurnKeypairAndIPAllocation(t *testing.T) {
	priv, pub, err := GenerateWireGuardKeypair()
	if err != nil {
		t.Fatalf("GenerateWireGuardKeypair failed: %v", err)
	}
	if len(priv) == 0 || len(pub) == 0 {
		t.Fatalf("expected non-empty keypair, got priv=%q pub=%q", priv, pub)
	}

	cid, err := GenerateClientID()
	if err != nil {
		t.Fatalf("GenerateClientID failed: %v", err)
	}
	if len(cid) != 32 {
		t.Fatalf("expected 32 hex chars client ID, got len=%d (%s)", len(cid), cid)
	}

	// Test NextFreeIP
	used := []string{"10.13.13.2/32", "10.13.13.3/32"}
	next, err := NextFreeIP(used, "10.13.13.0/24")
	if err != nil {
		t.Fatalf("NextFreeIP failed: %v", err)
	}
	if next != "10.13.13.4/32" {
		t.Errorf("expected 10.13.13.4/32, got %s", next)
	}
}

