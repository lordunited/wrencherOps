# The containers this workspace runs: rename, add or remove entries, then
# plan. Secret env vars go in container_secrets on the Variables tab.
containers = {
  "web" = {
    image = "nginx:1.27-alpine"
    ports = [{ internal = 80, external = 8081 }]
    volumes = [
      { container_path = "/usr/share/nginx/html", host_path = "/srv/web/html", read_only = true },
    ]
    healthcheck = ["CMD", "wget", "-qO-", "http://localhost/"]
  }
}
