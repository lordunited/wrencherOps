// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Produces a self-contained server in .next/standalone for the Docker image.
  output: "standalone",
  experimental: {
    // forbidden(): a page the user has no access to shows the no-access
    // page (app/(panel)/forbidden.tsx) with a 403.
    authInterrupts: true,
    // Requests pass through proxy.ts, which buffers a body only up to this
    // size (10 MB by default) and silently cuts the rest: an uploaded Ansible
    // project archive (up to 10 MB, base64-encoded) needs more.
    proxyClientMaxBodySize: "16mb",
  },
};

export default nextConfig;
