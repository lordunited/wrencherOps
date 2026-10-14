// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import type { Role } from "@/lib/api";

export const ROLE_INFO: Record<Role, string> = {
  admin: "Everything, in every tab, and manages users and access.",
  writer: "Exactly the access given below: view or write, per tab.",
  viewer: "Can look, never change: at most view, whatever is given.",
};
