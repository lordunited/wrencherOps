// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

// isImportAddress is whether to can be an import block's "to": a root
// module resource, type.name (e.g. aws_instance.web).
export function isImportAddress(to: string): boolean {
  return /^[A-Za-z][A-Za-z0-9_-]*\.[A-Za-z_][A-Za-z0-9_-]*$/.test(to);
}

// importBlock is an import block for the editor: Terraform's config-driven
// import (plan shows it, apply brings the object under management).
export function importBlock(to: string, id: string): string {
  return `import {\n  to = ${to}\n  id = ${JSON.stringify(id)}\n}\n`;
}
