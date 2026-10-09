package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"mikan/internal/panel/store/db"
)

type FreeTurnView struct {
	Configured bool   `json:"configured"`
	Enabled    bool   `json:"enabled"`
	ListenPort int    `json:"listen_port"`
	ObfProfile string `json:"obf_profile"`
	ObfKey     string `json:"obf_key"`
	WgPort     int    `json:"wg_port"`
	WgNetwork  string `json:"wg_network"`
	WgPubkey   string `json:"wg_pubkey"`
	PeersCount int    `json:"peers_count"`
}

type freeTurnOutput struct{ Body FreeTurnView }

type freeTurnGetInput struct {
	ID int64 `path:"id" minimum:"1"`
}

type freeTurnPatchInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body struct {
		Enabled    *bool   `json:"enabled,omitempty"`
		ListenPort *int    `json:"listen_port,omitempty" minimum:"1" maximum:"65535"`
		ObfProfile *string `json:"obf_profile,omitempty"`
		ObfKey     *string `json:"obf_key,omitempty"`
		WgPort     *int    `json:"wg_port,omitempty" minimum:"1" maximum:"65535"`
		WgNetwork  *string `json:"wg_network,omitempty"`
		WgPubkey   *string `json:"wg_pubkey,omitempty"`
	}
}

func (h *handlers) registerFreeTurn() {
	tags := []string{"node"}
	huma.Register(h.api, huma.Operation{OperationID: "get-node-freeturn", Method: http.MethodGet, Path: "/api/v1/nodes/{id}/freeturn", Summary: "FreeTurn (vk-turn-proxy) ноды", Tags: tags}, h.getFreeTurn)
	huma.Register(h.api, huma.Operation{OperationID: "update-node-freeturn", Method: http.MethodPatch, Path: "/api/v1/nodes/{id}/freeturn", Summary: "Настроить FreeTurn для ноды", Tags: tags}, h.patchFreeTurn)
	huma.Register(h.api, huma.Operation{OperationID: "delete-node-freeturn", Method: http.MethodDelete, Path: "/api/v1/nodes/{id}/freeturn", Summary: "Отключить FreeTurn на ноде", Tags: tags, DefaultStatus: http.StatusNoContent}, h.deleteFreeTurn)
}

func (h *handlers) getFreeTurn(ctx context.Context, in *freeTurnGetInput) (*freeTurnOutput, error) {
	ft, err := h.d.Store.Q.GetNodeFreeTurn(ctx, in.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return &freeTurnOutput{Body: FreeTurnView{
			Configured: false,
			Enabled:    false,
			ListenPort: 56392,
			ObfProfile: "rtpopus3",
			WgPort:     51820,
			WgNetwork:  "10.13.13.0/24",
		}}, nil
	}
	if err != nil {
		return nil, huma.Error500InternalServerError("get freeturn", err)
	}

	peers, _ := h.d.Store.Q.ListFreeTurnPeersForNode(ctx)
	return &freeTurnOutput{Body: FreeTurnView{
		Configured: true,
		Enabled:    ft.Enabled != 0,
		ListenPort: int(ft.ListenPort),
		ObfProfile: ft.ObfProfile,
		ObfKey:     ft.ObfKey,
		WgPort:     int(ft.WgPort),
		WgNetwork:  ft.WgNetwork,
		WgPubkey:   ft.WgPubkey,
		PeersCount: len(peers),
	}}, nil
}

func (h *handlers) patchFreeTurn(ctx context.Context, in *freeTurnPatchInput) (*freeTurnOutput, error) {
	now := h.d.Now().Unix()
	ft, err := h.d.Store.Q.GetNodeFreeTurn(ctx, in.ID)
	enabled := int64(1)
	listenPort := int64(56392)
	obfProfile := "rtpopus3"
	obfKey := ""
	wgPort := int64(51820)
	wgNetwork := "10.13.13.0/24"
	wgPubkey := ""
	createdAt := now

	if err == nil {
		enabled = ft.Enabled
		listenPort = ft.ListenPort
		obfProfile = ft.ObfProfile
		obfKey = ft.ObfKey
		wgPort = ft.WgPort
		wgNetwork = ft.WgNetwork
		wgPubkey = ft.WgPubkey
		createdAt = ft.CreatedAt
	}

	if in.Body.Enabled != nil {
		if *in.Body.Enabled {
			enabled = 1
		} else {
			enabled = 0
		}
	}
	if in.Body.ListenPort != nil {
		listenPort = int64(*in.Body.ListenPort)
	}
	if in.Body.ObfProfile != nil {
		obfProfile = *in.Body.ObfProfile
	}
	if in.Body.ObfKey != nil {
		obfKey = *in.Body.ObfKey
	}
	if in.Body.WgPort != nil {
		wgPort = int64(*in.Body.WgPort)
	}
	if in.Body.WgNetwork != nil {
		wgNetwork = *in.Body.WgNetwork
	}
	if in.Body.WgPubkey != nil {
		wgPubkey = *in.Body.WgPubkey
	}

	if err := h.d.Store.Q.SaveNodeFreeTurn(ctx, db.SaveNodeFreeTurnParams{
		NodeID:     in.ID,
		Enabled:    enabled,
		ListenPort: listenPort,
		ObfProfile: obfProfile,
		ObfKey:     obfKey,
		WgPort:     wgPort,
		WgNetwork:  wgNetwork,
		WgPubkey:   wgPubkey,
		CreatedAt:  createdAt,
		UpdatedAt:  now,
	}); err != nil {
		return nil, huma.Error500InternalServerError("save freeturn", err)
	}

	if h.d.Changes != nil {
		h.d.Changes.SlotsChanged()
	}

	return h.getFreeTurn(ctx, &freeTurnGetInput{ID: in.ID})
}

func (h *handlers) deleteFreeTurn(ctx context.Context, in *freeTurnGetInput) (*struct{}, error) {
	if err := h.d.Store.Q.DeleteNodeFreeTurn(ctx, in.ID); err != nil {
		return nil, huma.Error500InternalServerError("delete freeturn", err)
	}
	if h.d.Changes != nil {
		h.d.Changes.SlotsChanged()
	}
	return nil, nil
}
