// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

// Duplicated from lib/api.ts's SESSION_COOKIE_NAME rather than imported:
// that module pulls in next/headers and next/navigation, which aren't
// meant for the Proxy runtime's bundle.
const SESSION_COOKIE_NAME = "session";

// Optimistic check only: presence of the cookie, not validity. The real
// check happens server-side against the backend (see lib/api.ts fetchJSON),
// which redirects to /login on a 401. This just avoids rendering protected
// pages at all when there's obviously no session.
export function proxy(request: NextRequest) {
  if (!request.cookies.has(SESSION_COOKIE_NAME)) {
    // API calls get a 401 their caller can handle; redirecting them would
    // hand a script the login page's HTML instead of an answer.
    if (request.nextUrl.pathname.startsWith("/api/")) {
      return new NextResponse("not authenticated", { status: 401 });
    }
    return NextResponse.redirect(new URL("/login", request.url));
  }
  return NextResponse.next();
}

export const config = {
  matcher: ["/((?!login|api/auth|_next/static|_next/image|favicon.ico).*)"],
};
