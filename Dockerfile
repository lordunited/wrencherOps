# Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
# Author: Mohammadreza Masoumi, DevOps Engineer

# One Dockerfile, two images. docker-compose.yml picks each with `target:`.
#   backend - the Go API (plus the hashpassword and genkey helpers)
#   web     - the Next.js panel

# ---------- Go backend ----------
FROM golang:1.27-alpine AS backend-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server \
 && CGO_ENABLED=0 go build -o /out/hashpassword ./cmd/hashpassword \
 && CGO_ENABLED=0 go build -o /out/genkey ./cmd/genkey

FROM alpine:3.22 AS backend
# ansible-core runs the Ansible panel's playbooks (verified: 2.18 on 3.22);
# sshpass and the OpenSSH client let it log in with the servers' saved
# passwords.
#
# OpenTofu runs the Terraform panel's workspaces: Terraform's open-source
# fork, the same .tf language and providers, whose registry and downloads
# are reachable here — HashiCorp's provider registry answers "Content not
# available in your region" (verified), so terraform init couldn't fetch
# any provider. Providers are cached in /var/cache/panel-tofu (a volume).
RUN apk add --no-cache ca-certificates ansible-core sshpass openssh-client-default opentofu \
 && adduser -D -H -u 10001 app \
 && mkdir -p /var/cache/panel-tofu && chown app:app /var/cache/panel-tofu
COPY --from=backend-build /out/ /app/
USER app
EXPOSE 8080
ENTRYPOINT ["/app/server"]

# ---------- Next.js panel ----------
FROM node:22-alpine AS web-deps
WORKDIR /app
COPY web/package.json web/package-lock.json ./
RUN npm ci

FROM node:22-alpine AS web-build
WORKDIR /app
COPY --from=web-deps /app/node_modules ./node_modules
COPY web/ ./
ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build

FROM node:22-alpine AS web
WORKDIR /app
ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    HOSTNAME=0.0.0.0 \
    PORT=3000
COPY --from=web-build /app/public ./public
COPY --from=web-build --chown=node:node /app/.next/standalone ./
COPY --from=web-build --chown=node:node /app/.next/static ./.next/static
USER node
EXPOSE 3000
CMD ["node", "server.js"]
