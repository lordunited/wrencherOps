// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

//go:build linux

package main

import (
	"log"

	"golang.org/x/sys/unix"
)

// protectSecrets makes this process non-dumpable, which also makes its
// /proc/<pid> entries — environ included, where the database URL, the
// credential key and the GitLab token are — readable only by root. The
// Ansible panel runs playbooks as the same user, and a task delegated to
// localhost could otherwise read them from /proc/1/environ.
func protectSecrets() {
	if err := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); err != nil {
		log.Printf("could not make the process non-dumpable: %v", err)
	}
}
