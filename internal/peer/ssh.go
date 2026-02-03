package peer

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/tunnelmesh/tunnelmesh/internal/config"
	"github.com/tunnelmesh/tunnelmesh/internal/transport"
)

// HandleIncomingSSH accepts incoming SSH connections from the transport listener.
func (m *MeshNode) HandleIncomingSSH(ctx context.Context, listener transport.Listener) {
	for {
		conn, err := listener.Accept(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Error().Err(err).Msg("SSH accept error")
			continue
		}

		go m.handleIncomingConnection(ctx, conn, "SSH")
	}
}

// ensurePeerRoute fetches peer info from coordination server and ensures the route exists.
// Retries on failure with exponential backoff, then falls back to cached peer info.
// Returns the mesh IP for the peer (empty string if not found).
func (m *MeshNode) ensurePeerRoute(peerName string) string {
	if m.client == nil {
		// Try cache if no client available
		if meshIP, ok := m.GetCachedPeerMeshIP(peerName); ok {
			m.router.AddRoute(meshIP, peerName)
			log.Debug().
				Str("peer", peerName).
				Str("mesh_ip", meshIP).
				Msg("route added from cache (no client)")
			return meshIP
		}
		return ""
	}

	// Retry with exponential backoff
	const maxRetries = 5
	backoff := 500 * time.Millisecond

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		peers, err := m.client.ListPeers()
		if err != nil {
			lastErr = err
			log.Debug().
				Err(err).
				Str("peer", peerName).
				Int("attempt", attempt).
				Msg("failed to fetch peer info, retrying")
			if attempt < maxRetries {
				time.Sleep(backoff)
				backoff *= 2
			}
			continue
		}

		for _, peer := range peers {
			if peer.Name == peerName {
				// Add route for this peer's mesh IP
				m.router.AddRoute(peer.MeshIP, peer.Name)
				// Cache for future use
				m.CachePeerMeshIP(peer.Name, peer.MeshIP)
				log.Debug().
					Str("peer", peer.Name).
					Str("mesh_ip", peer.MeshIP).
					Msg("route added for incoming connection")

				// Also add authorized key if we have the SSH transport
				if peer.PublicKey != "" && m.SSHTransport != nil {
					pubKey, err := config.DecodePublicKey(peer.PublicKey)
					if err != nil {
						log.Warn().Err(err).Str("peer", peer.Name).Msg("failed to decode peer public key")
					} else {
						m.SSHTransport.AddAuthorizedKey(pubKey)
					}
				}
				return peer.MeshIP
			}
		}
		// Peer not in list - don't retry, it's not a transient error
		break
	}

	// All retries failed or peer not found, try cache
	if meshIP, ok := m.GetCachedPeerMeshIP(peerName); ok {
		m.router.AddRoute(meshIP, peerName)
		log.Debug().
			Str("peer", peerName).
			Str("mesh_ip", meshIP).
			Msg("route added from cache (retries exhausted)")
		return meshIP
	}

	if lastErr != nil {
		log.Warn().Err(lastErr).Str("peer", peerName).Msg("failed to fetch peer info for routing after retries")
	} else {
		log.Warn().Str("peer", peerName).Msg("peer not found on coordination server or in cache")
	}
	return ""
}
