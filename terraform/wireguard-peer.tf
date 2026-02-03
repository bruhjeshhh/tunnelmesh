# WireGuard concentrator peer running on a DigitalOcean droplet

variable "wg_peer_enabled" {
  description = "Enable the WireGuard peer droplet"
  type        = bool
  default     = true
}

variable "wg_peer_name" {
  description = "Name for the WireGuard peer"
  type        = string
  default     = "tm-net"
}

variable "wg_peer_size" {
  description = "Droplet size (s-1vcpu-512mb-10gb is $4/mo, s-1vcpu-1gb is $6/mo)"
  type        = string
  default     = "s-1vcpu-512mb-10gb" # Cheapest tier: $4/month
}

variable "wg_listen_port" {
  description = "WireGuard UDP listen port"
  type        = number
  default     = 51820
}

variable "ssh_key_fingerprint" {
  description = "SSH key fingerprint for droplet access"
  type        = string
  default     = ""
}

# SSH key for the droplet (optional - can use existing key)
data "digitalocean_ssh_key" "wg_peer" {
  count = var.ssh_key_fingerprint != "" ? 1 : 0
  name  = var.ssh_key_fingerprint
}

# The WireGuard peer droplet
resource "digitalocean_droplet" "wg_peer" {
  count = var.wg_peer_enabled ? 1 : 0

  name     = var.wg_peer_name
  region   = var.region
  size     = var.wg_peer_size
  image    = "ubuntu-24-04-x64"
  ssh_keys = var.ssh_key_fingerprint != "" ? [data.digitalocean_ssh_key.wg_peer[0].id] : []

  # Enable monitoring
  monitoring = true

  user_data = <<-EOF
    #!/bin/bash
    set -e

    # Update system
    apt-get update
    apt-get upgrade -y

    # Install dependencies
    apt-get install -y curl wget

    # Create tunnelmesh directories
    mkdir -p /etc/tunnelmesh
    mkdir -p /var/lib/tunnelmesh/wireguard

    # Download latest tunnelmesh binary
    ARCH=$(dpkg --print-architecture)
    curl -sL "https://github.com/${var.github_owner}/tunnelmesh/releases/latest/download/tunnelmesh-linux-$ARCH" -o /usr/local/bin/tunnelmesh
    chmod +x /usr/local/bin/tunnelmesh

    # Create peer configuration
    cat > /etc/tunnelmesh/peer.yaml <<CONF
    name: "${var.wg_peer_name}"
    server: "https://${var.subdomain}.${var.domain}"
    auth_token: "${var.auth_token}"
    ssh_port: 2222
    private_key: /etc/tunnelmesh/peer.key

    tun:
      enabled: true
      name: tun-mesh

    dns:
      enabled: true
      listen: "127.0.0.1:5353"

    wireguard:
      enabled: true
      listen_port: ${var.wg_listen_port}
      data_dir: /var/lib/tunnelmesh/wireguard
      endpoint: "${var.wg_peer_name}.${var.domain}:${var.wg_listen_port}"
    CONF

    # Create systemd service
    cat > /etc/systemd/system/tunnelmesh.service <<SERVICE
    [Unit]
    Description=TunnelMesh Peer
    After=network-online.target
    Wants=network-online.target

    [Service]
    Type=simple
    ExecStart=/usr/local/bin/tunnelmesh up --config /etc/tunnelmesh/peer.yaml
    Restart=always
    RestartSec=5

    # Security hardening
    NoNewPrivileges=false
    ProtectSystem=full
    ProtectHome=true

    [Install]
    WantedBy=multi-user.target
    SERVICE

    # Configure firewall
    ufw allow 22/tcp    # SSH
    ufw allow 2222/tcp  # TunnelMesh SSH
    ufw allow ${var.wg_listen_port}/udp  # WireGuard
    ufw --force enable

    # Enable IP forwarding for WireGuard
    echo "net.ipv4.ip_forward = 1" >> /etc/sysctl.conf
    sysctl -p

    # Enable and start the service
    systemctl daemon-reload
    systemctl enable tunnelmesh
    systemctl start tunnelmesh
  EOF

  tags = ["tunnelmesh", "wireguard", "peer"]
}

# Firewall for the WireGuard peer
resource "digitalocean_firewall" "wg_peer" {
  count = var.wg_peer_enabled ? 1 : 0

  name        = "${var.wg_peer_name}-firewall"
  droplet_ids = [digitalocean_droplet.wg_peer[0].id]

  # SSH access
  inbound_rule {
    protocol         = "tcp"
    port_range       = "22"
    source_addresses = ["0.0.0.0/0", "::/0"]
  }

  # TunnelMesh SSH tunnel port
  inbound_rule {
    protocol         = "tcp"
    port_range       = "2222"
    source_addresses = ["0.0.0.0/0", "::/0"]
  }

  # WireGuard UDP
  inbound_rule {
    protocol         = "udp"
    port_range       = tostring(var.wg_listen_port)
    source_addresses = ["0.0.0.0/0", "::/0"]
  }

  # Allow all outbound
  outbound_rule {
    protocol              = "tcp"
    port_range            = "1-65535"
    destination_addresses = ["0.0.0.0/0", "::/0"]
  }

  outbound_rule {
    protocol              = "udp"
    port_range            = "1-65535"
    destination_addresses = ["0.0.0.0/0", "::/0"]
  }

  outbound_rule {
    protocol              = "icmp"
    destination_addresses = ["0.0.0.0/0", "::/0"]
  }
}

# DNS record for the WireGuard peer
resource "digitalocean_record" "wg_peer" {
  count = var.wg_peer_enabled ? 1 : 0

  domain = var.domain
  type   = "A"
  name   = var.wg_peer_name
  value  = digitalocean_droplet.wg_peer[0].ipv4_address
  ttl    = 300
}

# Outputs for the WireGuard peer
output "wg_peer_ip" {
  description = "WireGuard peer public IP address"
  value       = var.wg_peer_enabled ? digitalocean_droplet.wg_peer[0].ipv4_address : null
}

output "wg_peer_hostname" {
  description = "WireGuard peer hostname"
  value       = var.wg_peer_enabled ? "${var.wg_peer_name}.${var.domain}" : null
}

output "wg_peer_endpoint" {
  description = "WireGuard endpoint for clients"
  value       = var.wg_peer_enabled ? "${var.wg_peer_name}.${var.domain}:${var.wg_listen_port}" : null
}
