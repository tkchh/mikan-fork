package nodesync

import (
	"database/sql"
	"slices"
	"testing"
	"time"

	"mikan/internal/panel/domain"
	"mikan/internal/panel/store/db"
)

func TestFreeTurnPeerAllowedLogic(t *testing.T) {
	now := time.Now()
	grants := domain.GrantsLeft{}

	tests := []struct {
		name        string
		user        db.User
		ftInboundID int64
		wantAllowed bool
	}{
		{
			name: "active user with traffic left",
			user: db.User{
				ID:           1,
				Status:       "active",
				TrafficLimit: sql.NullInt64{Int64: 100 * 1024 * 1024, Valid: true},
				UsedUp:       10 * 1024 * 1024,
				UsedDown:     10 * 1024 * 1024,
			},
			ftInboundID: 990001,
			wantAllowed: true,
		},
		{
			name: "expired user",
			user: db.User{
				ID:        2,
				Status:    "active",
				ExpiresAt: sql.NullInt64{Int64: now.Add(-1 * time.Hour).Unix(), Valid: true},
			},
			ftInboundID: 990001,
			wantAllowed: false,
		},
		{
			name: "traffic limit exhausted",
			user: db.User{
				ID:           3,
				Status:       "active",
				TrafficLimit: sql.NullInt64{Int64: 50 * 1024 * 1024, Valid: true},
				UsedUp:       30 * 1024 * 1024,
				UsedDown:     20 * 1024 * 1024,
			},
			ftInboundID: 990001,
			wantAllowed: false,
		},
		{
			name: "disabled user",
			user: db.User{
				ID:     4,
				Status: "disabled",
			},
			ftInboundID: 990001,
			wantAllowed: false,
		},
		{
			name: "inbounds includes FreeTurn",
			user: db.User{
				ID:       5,
				Status:   "active",
				Inbounds: sql.NullString{String: "[1, 990001]", Valid: true},
			},
			ftInboundID: 990001,
			wantAllowed: true,
		},
		{
			name: "inbounds excludes FreeTurn",
			user: db.User{
				ID:       6,
				Status:   "active",
				Inbounds: sql.NullString{String: "[1, 2]", Valid: true},
			},
			ftInboundID: 990001,
			wantAllowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userGrants := grants.Main(tt.user.ID)
			state := domain.State(tt.user, userGrants, now)
			allowed := false
			if domain.CanConnect(state) {
				allowed = true
				userInbounds := domain.DecodeInbounds(tt.user.Inbounds)
				if len(userInbounds) > 0 {
					if !slices.Contains(userInbounds, tt.ftInboundID) && !slices.Contains(userInbounds, int64(990001)) {
						allowed = false
					}
				}
			}
			if allowed != tt.wantAllowed {
				t.Errorf("got allowed=%v, want %v", allowed, tt.wantAllowed)
			}
		})
	}
}
