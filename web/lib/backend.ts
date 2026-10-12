// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { cookies } from "next/headers";
import { NextResponse } from "next/server";
import { SESSION_COOKIE_NAME } from "@/lib/api";

const API_BASE_URL = process.env.API_BASE_URL ?? "http://localhost:8080";

// Forwards a request to the Go backend with the caller's session cookie
// attached, so the backend's own auth guard applies. Used by every
// app/api/** route handler that just proxies through to Go.
export async function proxyToBackend(path: string, init?: RequestInit): Promise<Response> {
  const cookieStore = await cookies();
  const sessionCookie = cookieStore.get(SESSION_COOKIE_NAME);

  return fetch(`${API_BASE_URL}${path}`, {
    ...init,
    cache: "no-store",
    headers: {
      ...(init?.headers ?? {}),
      ...(sessionCookie ? { Cookie: `${SESSION_COOKIE_NAME}=${sessionCookie.value}` } : {}),
    },
  });
}

// Statuses whose spec forbids a body — Response throws if given one, even "".
const NULL_BODY_STATUSES = new Set([204, 205, 304]);

// Copies a backend Response's status and JSON body into a NextResponse.
export async function relayJSON(res: Response): Promise<NextResponse> {
  if (NULL_BODY_STATUSES.has(res.status)) {
    return new NextResponse(null, { status: res.status });
  }
  const body = await res.text();
  return new NextResponse(body, {
    status: res.status,
    headers: { "Content-Type": "application/json" },
  });
}
