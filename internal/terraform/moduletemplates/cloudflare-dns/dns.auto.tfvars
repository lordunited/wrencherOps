# The DNS records this workspace manages, by zone: add, change or remove
# entries, then plan. Each record's key is unique in its zone; name
# defaults to the key, and @ is the zone apex.
zones = {
  "example.com" = {
    records = {
      "apex" = { name = "@", type = "A", content = "203.0.113.10", proxied = true }
      "www"  = { type = "CNAME", content = "example.com", proxied = true }
      "mail" = { name = "@", type = "MX", content = "mail.example.com", priority = 10 }
      "spf"  = { name = "@", type = "TXT", content = "\"v=spf1 mx -all\"" }
    }
  }
}
