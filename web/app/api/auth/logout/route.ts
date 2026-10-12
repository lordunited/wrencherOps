// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { cookies } from "next/headers";
import { NextResponse } from "next/server";
import { SESSION_COOKIE_NAME } from "@/lib/api";

const API_BASE_URL = process.env.API_BASE_URL ?? "http://localhost:8080";

export async function POST() {
  const cookieStore = await cookies();
  const sessionCookie = cookieStore.get(SESSION_COOKIE_NAME);

  const backendRes = await fetch(`${API_BASE_URL}/api/auth/logout`, {
    method: "POST",
    headers: sessionCookie
      ? { Cookie: `${SESSION_COOKIE_NAME}=${sessionCookie.value}` }
      : undefined,
  });

  const response = new NextResponse(null, { status: backendRes.status });
  const setCookie = backendRes.headers.get("set-cookie");
  if (setCookie) {
    response.headers.set("set-cookie", setCookie);
  } else {
    response.cookies.delete(SESSION_COOKIE_NAME);
  }
  return response;
}
