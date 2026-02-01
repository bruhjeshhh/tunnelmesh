# Reference existing DigitalOcean domain
data "digitalocean_domain" "main" {
  name = var.domain
}

# Create DNS record pointing to the App Platform app
resource "digitalocean_record" "coord" {
  domain = data.digitalocean_domain.main.id
  type   = "CNAME"
  name   = var.subdomain
  value  = "${digitalocean_app.tunnelmesh_coord.default_ingress}."
  ttl    = 300
}
