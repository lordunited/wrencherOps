// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package deploy

import (
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"devops-wrapper/internal/sshexec"
)

// ProjectFile is a file of a custom compose project, written into the
// deploy folder on every deploy. The panel is its source of truth, so
// unlike a SeedFile it's always overwritten.
type ProjectFile struct {
	Path    string `json:"path"` // relative to the deploy folder
	Content string `json:"content"`
}

// PanelFilesList, in the deploy folder, lists the project files the panel
// wrote there. A file later removed from the project is deleted from the
// server on the next deploy — but only one this list names, so nothing put
// in the folder by hand is ever touched.
const PanelFilesList = ".panel-files"

// Limits on project files: they're stored with the app and sent over SSH
// on every deploy, so this is for config files and scripts, not data.
const (
	MaxProjectFiles     = 100
	MaxProjectFileBytes = 1 << 20
	MaxProjectBytes     = 8 << 20
)

var projectPathRe = regexp.MustCompile(`^[A-Za-z0-9._@+-][A-Za-z0-9._@+/-]*$`)

// ComposeFileNames are the file names docker compose finds on its own, in
// its order of preference.
var ComposeFileNames = []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"}

// IsComposeFileName reports whether name is one docker compose picks up by
// itself. A project file may not use one: it would compete with the
// project's compose file for which one compose reads.
func IsComposeFileName(name string) bool {
	for _, n := range ComposeFileNames {
		if name == n {
			return true
		}
	}
	return false
}

// ValidProjectPath checks a project file's path: relative, inside the
// deploy folder, and not one of the files the panel itself manages.
func ValidProjectPath(p string) error {
	switch {
	case !projectPathRe.MatchString(p) || strings.Contains(p, "//") || strings.HasSuffix(p, "/"):
		return fmt.Errorf("%q isn't a valid file path (letters, digits, . _ - @ + and / between folders)", p)
	case path.Clean(p) != p || p == ".." || strings.HasPrefix(p, "../") || strings.Contains(p, "/../"):
		return fmt.Errorf("%q must stay inside the deploy folder", p)
	case IsComposeFileName(p) || p == PanelFilesList:
		return fmt.Errorf("%q is managed by the panel", p)
	}
	return nil
}

// ValidateProjectFiles checks a whole set of project files: valid, unique
// paths, text content, and the size limits.
func ValidateProjectFiles(files []ProjectFile) error {
	if len(files) > MaxProjectFiles {
		return fmt.Errorf("a project can have at most %d files", MaxProjectFiles)
	}
	seen := map[string]bool{}
	total := 0
	for _, f := range files {
		if err := ValidProjectPath(f.Path); err != nil {
			return err
		}
		if seen[f.Path] {
			return fmt.Errorf("%s is listed twice", f.Path)
		}
		seen[f.Path] = true
		if len(f.Content) > MaxProjectFileBytes {
			return fmt.Errorf("%s is larger than %d KiB; keep data out of the project and in a volume", f.Path, MaxProjectFileBytes>>10)
		}
		if !utf8.ValidString(f.Content) || strings.ContainsRune(f.Content, 0) {
			return fmt.Errorf("%s isn't a text file; only text files can be part of a project", f.Path)
		}
		total += len(f.Content)
	}
	if total > MaxProjectBytes {
		return fmt.Errorf("the project's files add up to more than %d MiB", MaxProjectBytes>>20)
	}
	return nil
}

// writeProjectFiles removes the files the panel wrote last time that are no
// longer in the project, writes every current one, and records the new
// list. Paths come from ValidateProjectFiles, and old ones are re-checked
// before anything is removed, so nothing outside the deploy folder can be
// touched even if the list on the server were edited.
func writeProjectFiles(job *Job, client *sshexec.Client, out io.Writer, deployDir string, files []ProjectFile) bool {
	keep := map[string]bool{}
	var paths []string
	for _, f := range files {
		keep[f.Path] = true
		paths = append(paths, f.Path)
	}
	sort.Strings(paths)

	previous, _, _ := client.Output(sshexec.AsRoot("cat " + sshexec.ShellQuote(deployDir+"/"+PanelFilesList) + " 2>/dev/null"))
	for _, p := range strings.Split(previous, "\n") {
		p = strings.TrimSpace(p)
		if p == "" || keep[p] || ValidProjectPath(p) != nil {
			continue
		}
		if err := client.Run(sshexec.AsRoot("rm -f "+sshexec.ShellQuote(deployDir+"/"+p)), out); err != nil {
			job.appendLog("Failed to remove " + p + ": " + err.Error() + "\n")
			job.setStatus(StatusFailed)
			return false
		}
		job.appendLog("Removed " + p + " (no longer in the project).\n")
	}

	for _, f := range files {
		if err := client.WriteFile(deployDir+"/"+f.Path, []byte(f.Content), out); err != nil {
			job.appendLog("Failed to write " + f.Path + ": " + err.Error() + "\n")
			job.setStatus(StatusFailed)
			return false
		}
		job.appendLog("Wrote " + f.Path + ".\n")
	}

	list := strings.Join(paths, "\n")
	if list != "" {
		list += "\n"
	}
	if err := client.WriteFile(deployDir+"/"+PanelFilesList, []byte(list), out); err != nil {
		job.appendLog("Failed to record the project's files: " + err.Error() + "\n")
		job.setStatus(StatusFailed)
		return false
	}
	return true
}
