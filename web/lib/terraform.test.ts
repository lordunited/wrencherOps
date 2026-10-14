// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { describe, expect, it } from "vitest";
import { CONCEPT_ACTIONS, importBlock, isImportAddress, tfStatusRole, TF_KIND } from "@/lib/terraform";

describe("concept actions", () => {
  // The backend's TestEveryConceptCanBeUsed lists the same 11 plus
  // Providers, which has its own picker.
  it("cover the run-side concepts", () => {
    expect(Object.keys(CONCEPT_ACTIONS).sort()).toEqual(
      ["Apply", "Destroy", "Import", "Init", "Plan", "Sentinel", "State File", "TFC/TFE", "Taint", "Workflows", "Workspace Variables"].sort(),
    );
  });
  it("let only writers run or change things", () => {
    for (const [name, a] of Object.entries(CONCEPT_ACTIONS)) {
      expect(a.write, name).toBe(["plan", "destroy-plan", "apply", "replace", "import"].includes(a.action));
    }
  });
});

describe("run labels", () => {
  it("name each kind and status", () => {
    expect(TF_KIND["destroy-plan"]).toBe("destroy plan");
    expect(tfStatusRole("failed")).toBe("critical");
    expect(tfStatusRole("running")).toBe("warning");
  });
});

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
