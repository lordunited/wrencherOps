// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { NoAccess } from "@/components/Access";

// Shown, with a 403, when the backend refuses a page's data: the user has
// no access to that tab.
export default function Forbidden() {
  return <NoAccess />;
}
