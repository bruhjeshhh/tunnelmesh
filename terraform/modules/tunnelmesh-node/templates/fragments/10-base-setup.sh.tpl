# Make apt fully non-interactive
export DEBIAN_FRONTEND=noninteractive
export NEEDRESTART_MODE=a

# Update system (keep existing configs on conflicts)
apt-get update
apt-get -o Dpkg::Options::="--force-confold" -o Dpkg::Options::="--force-confdef" upgrade -y

# Install base dependencies
apt-get install -y -q curl wget jq

%{ if coordinator_enabled && ssl_enabled ~}
# Install nginx and certbot for SSL termination
apt-get install -y -q nginx certbot python3-certbot-nginx
%{ endif ~}

# Create tunnelmesh directories
mkdir -p /etc/tunnelmesh
mkdir -p /var/lib/tunnelmesh

%{ if wireguard_enabled && peer_enabled ~}
mkdir -p /var/lib/tunnelmesh/wireguard
%{ endif ~}
