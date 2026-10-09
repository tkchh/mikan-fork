package node

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"mikan/internal/nodeapi"
)

const (
	defaultWgIface     = "ft-wg0"
	defaultClientsPath = "/opt/free-turn-proxy/auth/clients.json"
	pollInterval       = 3 * time.Second
)

type clientInfo struct {
	Comment string `json:"comment,omitempty"`
}

type clientsDB struct {
	Clients map[string]clientInfo `json:"clients"`
}

// FreeTurnManager manages the free-turn-proxy clients and the ft-wg0 WireGuard interface.
type FreeTurnManager struct {
	mu          sync.Mutex
	log         *slog.Logger
	dataDir     string
	clientsPath string
	ifaceName   string
	wgClient    *wgctrl.Client

	config    *nodeapi.FreeTurnConfig
	slotByPub map[string]string // hex(pubkey) -> slot_name
	prevRx    map[string]int64  // hex(pubkey) -> last rx
	prevTx    map[string]int64  // hex(pubkey) -> last tx

	pollCtx    context.Context
	pollCancel context.CancelFunc
}

func NewFreeTurnManager(dataDir string, log *slog.Logger) *FreeTurnManager {
	clientsPath := os.Getenv("FREETURN_CLIENTS_FILE")
	if clientsPath == "" {
		clientsPath = defaultClientsPath
	}
	iface := os.Getenv("FREETURN_WG_IFACE")
	if iface == "" {
		iface = defaultWgIface
	}

	var wgc *wgctrl.Client
	c, err := wgctrl.New()
	if err == nil {
		wgc = c
	} else if log != nil {
		log.Debug("freeturn: wgctrl not available in this environment", "err", err)
	}

	return &FreeTurnManager{
		log:         log,
		dataDir:     dataDir,
		clientsPath: clientsPath,
		ifaceName:   iface,
		wgClient:    wgc,
		slotByPub:   make(map[string]string),
		prevRx:      make(map[string]int64),
		prevTx:      make(map[string]int64),
	}
}

// Apply updates the active FreeTurn configuration and peers.
func (m *FreeTurnManager) Apply(cfg *nodeapi.FreeTurnConfig, reg *Registry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.pollCancel != nil {
		m.pollCancel()
		m.pollCancel = nil
	}

	m.config = cfg
	if cfg == nil || !cfg.Enabled {
		return nil
	}

	// 1. Update clients.json for free-turn-proxy
	if err := m.syncClientsFile(cfg.Peers); err != nil {
		if m.log != nil {
			m.log.Warn("freeturn: failed to sync clients.json", "err", err)
		}
	}

	// 2. Sync WireGuard peers
	m.syncWgPeers(cfg.Peers)

	// 3. Start background stats polling
	ctx, cancel := context.WithCancel(context.Background())
	m.pollCtx = ctx
	m.pollCancel = cancel
	go m.pollLoop(ctx, reg)

	return nil
}

// syncClientsFile writes the active client IDs to clients.json atomically.
func (m *FreeTurnManager) syncClientsFile(peers []nodeapi.FreeTurnPeer) error {
	db := clientsDB{Clients: make(map[string]clientInfo)}
	for _, p := range peers {
		if p.Allowed && p.ClientID != "" {
			db.Clients[p.ClientID] = clientInfo{Comment: p.SlotName}
		}
	}

	b, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(m.clientsPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmpFile := m.clientsPath + ".tmp"
	if err := os.WriteFile(tmpFile, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpFile, m.clientsPath)
}

// syncWgPeers configures peers on ft-wg0 WireGuard interface without downtime.
func (m *FreeTurnManager) syncWgPeers(peers []nodeapi.FreeTurnPeer) {
	if m.wgClient == nil {
		return
	}

	var peerConfigs []wgtypes.PeerConfig
	m.slotByPub = make(map[string]string)

	for _, p := range peers {
		key, err := wgtypes.ParseKey(p.PublicKey)
		if err != nil {
			continue
		}
		keyHex := key.String()
		m.slotByPub[keyHex] = p.SlotName

		if !p.Allowed {
			// Remove peer immediately if quota is exhausted or user is disabled
			peerConfigs = append(peerConfigs, wgtypes.PeerConfig{
				PublicKey: key,
				Remove:    true,
			})
			continue
		}

		ipPrefix, err := netip.ParsePrefix(p.AllowedIP)
		var ipNet net.IPNet
		if err == nil {
			ipNet = net.IPNet{
				IP:   net.IP(ipPrefix.Addr().AsSlice()),
				Mask: net.CIDRMask(ipPrefix.Bits(), ipPrefix.Addr().BitLen()),
			}
		}

		peerConfigs = append(peerConfigs, wgtypes.PeerConfig{
			PublicKey:         key,
			AllowedIPs:        []net.IPNet{ipNet},
			ReplaceAllowedIPs: true,
		})
	}

	if len(peerConfigs) > 0 {
		err := m.wgClient.ConfigureDevice(m.ifaceName, wgtypes.Config{
			Peers: peerConfigs,
		})
		if err != nil && m.log != nil {
			m.log.Warn("freeturn: configure WireGuard peers failed", "err", err)
		}
	}
}

// pollLoop continuously collects traffic statistics from ft-wg0 and updates the Registry.
func (m *FreeTurnManager) pollLoop(ctx context.Context, reg *Registry) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.pollStats(reg)
		}
	}
}

func (m *FreeTurnManager) pollStats(reg *Registry) {
	if m.wgClient == nil || reg == nil {
		return
	}

	dev, err := m.wgClient.Device(m.ifaceName)
	if err != nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, peer := range dev.Peers {
		keyHex := peer.PublicKey.String()
		slotName, ok := m.slotByPub[keyHex]
		if !ok {
			continue
		}

		// Calculate rx/tx deltas
		rx := peer.ReceiveBytes
		tx := peer.TransmitBytes

		lastRx := m.prevRx[keyHex]
		lastTx := m.prevTx[keyHex]

		var upDelta, downDelta int64
		if rx >= lastRx && lastRx > 0 {
			upDelta = rx - lastRx // from client to server = upload
		}
		if tx >= lastTx && lastTx > 0 {
			downDelta = tx - lastTx // from server to client = download
		}

		m.prevRx[keyHex] = rx
		m.prevTx[keyHex] = tx

		if upDelta > 0 || downDelta > 0 {
			reg.RecordPeerTraffic(slotName, upDelta, downDelta)
		}
	}
}

// Close stops the manager and releases WireGuard client.
func (m *FreeTurnManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.pollCancel != nil {
		m.pollCancel()
		m.pollCancel = nil
	}
	if m.wgClient != nil {
		return m.wgClient.Close()
	}
	return nil
}
