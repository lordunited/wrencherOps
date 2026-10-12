// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

//go:build !linux

package main

// protectSecrets is Linux-only (see nodump_linux.go); the panel's backend
// runs on Linux, this keeps it building elsewhere.
func protectSecrets() {}
