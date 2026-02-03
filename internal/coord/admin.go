package coord

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/tunnelmesh/tunnelmesh/internal/coord/web"
	"github.com/tunnelmesh/tunnelmesh/internal/coord/wireguard"
	"github.com/tunnelmesh/tunnelmesh/pkg/proto"
)

// AdminOverview is the response for the admin overview endpoint.
type AdminOverview struct {
	ServerUptime    string          `json:"server_uptime"`
	ServerVersion   string          `json:"server_version"`
	TotalPeers      int             `json:"total_peers"`
	OnlinePeers     int             `json:"online_peers"`
	TotalHeartbeats uint64          `json:"total_heartbeats"`
	MeshCIDR        string          `json:"mesh_cidr"`
	DomainSuffix    string          `json:"domain_suffix"`
	Peers           []AdminPeerInfo `json:"peers"`
}

// AdminPeerInfo contains peer information for the admin UI.
type AdminPeerInfo struct {
	Name                string           `json:"name"`
	MeshIP              string           `json:"mesh_ip"`
	PublicIPs           []string         `json:"public_ips"`
	PrivateIPs          []string         `json:"private_ips"`
	SSHPort             int              `json:"ssh_port"`
	UDPPort             int              `json:"udp_port"`
	UDPExternalAddr4    string           `json:"udp_external_addr4,omitempty"`
	UDPExternalAddr6    string           `json:"udp_external_addr6,omitempty"`
	LastSeen            time.Time        `json:"last_seen"`
	Online              bool             `json:"online"`
	Connectable         bool             `json:"connectable"`
	BehindNAT           bool             `json:"behind_nat"`
	RegisteredAt        time.Time        `json:"registered_at"`
	HeartbeatCount      uint64           `json:"heartbeat_count"`
	Stats               *proto.PeerStats `json:"stats,omitempty"`
	BytesSentRate       float64          `json:"bytes_sent_rate"`
	BytesReceivedRate   float64          `json:"bytes_received_rate"`
	PacketsSentRate     float64          `json:"packets_sent_rate"`
	PacketsReceivedRate float64          `json:"packets_received_rate"`
	Version             string           `json:"version,omitempty"`
}

// handleAdminOverview returns the admin overview data.
func (s *Server) handleAdminOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.peersMu.RLock()
	defer s.peersMu.RUnlock()

	now := time.Now()
	onlineThreshold := 2 * time.Minute

	overview := AdminOverview{
		ServerUptime:    time.Since(s.serverStats.startTime).Round(time.Second).String(),
		ServerVersion:   s.version,
		TotalPeers:      len(s.peers),
		TotalHeartbeats: s.serverStats.totalHeartbeats,
		MeshCIDR:        s.cfg.MeshCIDR,
		DomainSuffix:    s.cfg.DomainSuffix,
		Peers:           make([]AdminPeerInfo, 0, len(s.peers)),
	}

	for _, info := range s.peers {
		online := now.Sub(info.peer.LastSeen) < onlineThreshold
		if online {
			overview.OnlinePeers++
		}

		peerInfo := AdminPeerInfo{
			Name:           info.peer.Name,
			MeshIP:         info.peer.MeshIP,
			PublicIPs:      info.peer.PublicIPs,
			PrivateIPs:     info.peer.PrivateIPs,
			SSHPort:        info.peer.SSHPort,
			UDPPort:        info.peer.UDPPort,
			LastSeen:       info.peer.LastSeen,
			Online:         online,
			Connectable:    info.peer.Connectable,
			BehindNAT:      info.peer.BehindNAT,
			RegisteredAt:   info.registeredAt,
			HeartbeatCount: info.heartbeatCount,
			Stats:          info.stats,
			Version:        info.peer.Version,
		}

		// Get UDP endpoint addresses if available
		if s.holePunch != nil {
			if ep, ok := s.holePunch.GetEndpoint(info.peer.Name); ok {
				peerInfo.UDPExternalAddr4 = ep.ExternalAddr4
				peerInfo.UDPExternalAddr6 = ep.ExternalAddr6
			}
		}

		// Calculate rates if we have previous stats
		if info.prevStats != nil && info.stats != nil && !info.lastStatsTime.IsZero() {
			// Rate is calculated as delta over 30 seconds (heartbeat interval)
			peerInfo.BytesSentRate = float64(info.stats.BytesSent-info.prevStats.BytesSent) / 30.0
			peerInfo.BytesReceivedRate = float64(info.stats.BytesReceived-info.prevStats.BytesReceived) / 30.0
			peerInfo.PacketsSentRate = float64(info.stats.PacketsSent-info.prevStats.PacketsSent) / 30.0
			peerInfo.PacketsReceivedRate = float64(info.stats.PacketsReceived-info.prevStats.PacketsReceived) / 30.0
		}

		overview.Peers = append(overview.Peers, peerInfo)
	}

	// Sort peers by mesh IP for consistent ordering
	sort.Slice(overview.Peers, func(i, j int) bool {
		ipI := net.ParseIP(overview.Peers[i].MeshIP)
		ipJ := net.ParseIP(overview.Peers[j].MeshIP)
		if ipI == nil || ipJ == nil {
			return overview.Peers[i].MeshIP < overview.Peers[j].MeshIP
		}
		return bytes.Compare(ipI.To16(), ipJ.To16()) < 0
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(overview)
}

// setupAdminRoutes registers the admin API routes and static file server.
func (s *Server) setupAdminRoutes() {
	// API endpoints
	s.mux.HandleFunc("/admin/api/overview", s.handleAdminOverview)

	// WireGuard client management endpoints (if enabled)
	if s.cfg.WireGuard.Enabled {
		s.mux.HandleFunc("/admin/api/wireguard/clients", s.handleWGClients)
		s.mux.HandleFunc("/admin/api/wireguard/clients/", s.handleWGClientByID)
	}

	// Serve embedded static files
	staticFS, _ := fs.Sub(web.Assets, ".")
	fileServer := http.FileServer(http.FS(staticFS))

	// Serve index.html at /admin/ and /admin
	s.mux.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusMovedPermanently)
	})
	s.mux.Handle("/admin/", http.StripPrefix("/admin/", fileServer))
}

// handleWGClients handles GET (list) and POST (create) for WireGuard clients.
func (s *Server) handleWGClients(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleWGClientsList(w, r)
	case http.MethodPost:
		s.handleWGClientCreate(w, r)
	default:
		s.jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleWGClientsList returns all WireGuard clients.
func (s *Server) handleWGClientsList(w http.ResponseWriter, _ *http.Request) {
	clients := s.wgStore.List()

	// Sort by mesh IP for consistent ordering
	sort.Slice(clients, func(i, j int) bool {
		return clients[i].MeshIP < clients[j].MeshIP
	})

	resp := wireguard.ClientListResponse{
		Clients:               clients,
		ConcentratorPublicKey: "", // Will be set when concentrator registers
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleWGClientCreate creates a new WireGuard client.
func (s *Server) handleWGClientCreate(w http.ResponseWriter, r *http.Request) {
	var req wireguard.CreateClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	client, privateKey, err := s.wgStore.CreateWithPrivateKey(req.Name)
	if err != nil {
		s.jsonError(w, "failed to create client: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Add to DNS cache
	s.peersMu.Lock()
	s.dnsCache[client.DNSName] = client.MeshIP
	s.peersMu.Unlock()

	// Generate config and QR code
	configParams := wireguard.ClientConfigParams{
		ClientPrivateKey: privateKey,
		ClientMeshIP:     client.MeshIP,
		ServerPublicKey:  "", // Will be set when concentrator registers its public key
		ServerEndpoint:   s.cfg.WireGuard.Endpoint,
		DNSServer:        "", // Optional: could be set to a DNS server in the mesh
		MeshCIDR:         s.cfg.MeshCIDR,
		MTU:              1420,
	}

	configStr := wireguard.GenerateClientConfig(configParams)

	qrCode, err := wireguard.GenerateQRCodeDataURL(configStr, 256)
	if err != nil {
		// QR generation failed, but we can still return the config
		qrCode = ""
	}

	resp := wireguard.CreateClientResponse{
		Client:     *client,
		PrivateKey: privateKey,
		Config:     configStr,
		QRCode:     qrCode,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

// handleWGClientByID handles GET, PATCH, DELETE for a specific WireGuard client.
func (s *Server) handleWGClientByID(w http.ResponseWriter, r *http.Request) {
	// Extract client ID from path
	id := strings.TrimPrefix(r.URL.Path, "/admin/api/wireguard/clients/")
	if id == "" {
		s.jsonError(w, "client ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		client, err := s.wgStore.Get(id)
		if err != nil {
			s.jsonError(w, "client not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client)

	case http.MethodPatch:
		var req wireguard.UpdateClientRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if err := req.Validate(); err != nil {
			s.jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}

		client, err := s.wgStore.Update(id, &req)
		if err != nil {
			s.jsonError(w, "client not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client)

	case http.MethodDelete:
		// Get client first to remove from DNS cache
		client, err := s.wgStore.Get(id)
		if err != nil {
			s.jsonError(w, "client not found", http.StatusNotFound)
			return
		}

		if err := s.wgStore.Delete(id); err != nil {
			s.jsonError(w, "failed to delete client", http.StatusInternalServerError)
			return
		}

		// Remove from DNS cache
		s.peersMu.Lock()
		delete(s.dnsCache, client.DNSName)
		s.peersMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	default:
		s.jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
