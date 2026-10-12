// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { describe, expect, it } from "vitest";
import { importBlock, isImportAddress } from "@/lib/terraform";

describe("import blocks", () => {
  it("accept only a root resource address", () => {
    expect(isImportAddress("aws_instance.web")).toBe(true);
    expect(isImportAddress("random_id.x_1")).toBe(true);
    for (const bad of ["", "aws_instance", "module.a.aws_instance.web", "data.aws_ami.x.y", "aws_instance.web[0]", "a.b; rm"]) {
      expect(isImportAddress(bad)).toBe(false);
    }
  });
  it("quote the ID", () => {
    expect(importBlock("aws_instance.web", 'i-0abc"x')).toBe('import {\n  to = aws_instance.web\n  id = "i-0abc\\"x"\n}\n');
  });
});
