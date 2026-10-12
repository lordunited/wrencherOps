// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import "testing"

func TestPromAPIPathAllowlist(t *testing.T) {
	allowed := []string{
		"v1/labels", "v1/series", "v1/metadata", "v1/query", "v1/status/flags",
		"v1/label/__name__/values", "v1/label/job/values", "v1/format_query",
	}
	for _, p := range allowed {
		if !promAPIPath.MatchString(p) {
			t.Errorf("%s should be allowed", p)
		}
	}
	denied := []string{
		"v1/admin/tsdb/delete_series", "v1/admin/tsdb/snapshot", "-/reload", "-/quit",
		"v1/label/../admin/values", "v1/label/a/b/values", "v1/querys", "v1/query/../../admin", "",
	}
	for _, p := range denied {
		if promAPIPath.MatchString(p) {
			t.Errorf("%s should be refused", p)
		}
	}
}
