// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package ansible

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// maxArchive bounds an uploaded archive, and maxUnpacked what it may unpack
// to (before non-text files are dropped), so a zip bomb can't fill memory.
const (
	maxArchive  = 10 << 20
	maxUnpacked = 64 << 20
)

// errAmbiguousEntry is DetectEntry finding several plays and no
// conventional name: the uploader picks one.
var errAmbiguousEntry = errors.New("several files could be the playbook to run")

// isPlay reports whether a YAML file is a playbook: a list of plays, each
// with hosts (or an import_playbook).
func isPlay(content string) bool {
	var plays []map[string]any
	if yaml.Unmarshal([]byte(content), &plays) != nil || len(plays) == 0 {
		return false
	}
	for _, p := range plays {
		_, hosts := p["hosts"]
		_, imp := p["import_playbook"]
		_, imp2 := p["ansible.builtin.import_playbook"]
		if !hosts && !imp && !imp2 {
			return false
		}
	}
	return true
}

// DetectEntry picks the file to run from an uploaded project: playbook.yml,
// else site.yml, else main.yml (as the conventions go), else the only
// top-level YAML file that is a playbook. candidates are every top-level
// playbook, for the uploader to choose from when it can't tell.
func DetectEntry(files []File) (entry string, candidates []string, err error) {
	byPath := map[string]string{}
	for _, f := range files {
		byPath[f.Path] = f.Content
		if !strings.Contains(f.Path, "/") && (strings.HasSuffix(f.Path, ".yml") || strings.HasSuffix(f.Path, ".yaml")) && isPlay(f.Content) {
			candidates = append(candidates, f.Path)
		}
	}
	sort.Strings(candidates)
	for _, name := range []string{"playbook.yml", "playbook.yaml", "site.yml", "site.yaml", "main.yml", "main.yaml"} {
		if c, ok := byPath[name]; ok && strings.TrimSpace(c) != "" {
			return name, candidates, nil
		}
	}
	switch len(candidates) {
	case 0:
		return "", nil, fmt.Errorf("no playbook found: none of the top-level YAML files is a list of plays with hosts")
	case 1:
		return candidates[0], candidates, nil
	}
	return "", candidates, errAmbiguousEntry
}

// junk is what an archive or folder carries that isn't part of a playbook.
func junk(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		switch seg {
		case ".git", "__MACOSX", ".idea", ".vscode", "__pycache__", "node_modules", ".DS_Store":
			return true
		}
	}
	return strings.HasPrefix(path.Base(p), "._")
}

// ExtractArchive unpacks a .zip, .tar, .tar.gz or .tgz into playbook files.
// Paths that would escape the folder, links and devices are refused; files
// that aren't text (binaries, images) or are too big are left out and
// listed in notes. A single top-level folder (how most archives come) is
// stripped, so its contents are the playbook's root.
func ExtractArchive(filename string, data []byte) (files []File, notes []string, err error) {
	files, notes, err = extract(filename, data)
	if err != nil {
		return nil, nil, err
	}
	files, more := CleanUpload(files)
	return files, append(notes, more...), nil
}

func extract(filename string, data []byte) (files []File, notes []string, err error) {
	if len(data) > maxArchive {
		return nil, nil, fmt.Errorf("the archive is over %d MiB", maxArchive>>20)
	}
	type entry struct {
		name string
		read func() ([]byte, error)
		size int64
	}
	var entries []entry
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, nil, fmt.Errorf("not a readable zip: %v", err)
		}
		for _, f := range zr.File {
			if f.FileInfo().IsDir() {
				continue
			}
			if !f.Mode().IsRegular() {
				notes = append(notes, f.Name+": not a regular file (a link?), left out")
				continue
			}
			f := f
			entries = append(entries, entry{f.Name, func() ([]byte, error) {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(io.LimitReader(rc, maxFileBytes+1))
			}, int64(f.UncompressedSize64)})
		}
	case strings.HasSuffix(lower, ".tar"), strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		var r io.Reader = bytes.NewReader(data)
		if !strings.HasSuffix(lower, ".tar") {
			gz, err := gzip.NewReader(r)
			if err != nil {
				return nil, nil, fmt.Errorf("not a readable gzip: %v", err)
			}
			r = gz
		}
		tr := tar.NewReader(io.LimitReader(r, maxUnpacked))
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, nil, fmt.Errorf("reading the archive: %v", err)
			}
			switch h.Typeflag {
			case tar.TypeDir, tar.TypeXGlobalHeader:
				continue
			case tar.TypeReg:
			default:
				notes = append(notes, h.Name+": not a regular file (a link?), left out")
				continue
			}
			b, err := io.ReadAll(io.LimitReader(tr, maxFileBytes+1))
			if err != nil {
				return nil, nil, err
			}
			entries = append(entries, entry{h.Name, func() ([]byte, error) { return b, nil }, int64(len(b))})
		}
	default:
		return nil, nil, fmt.Errorf("upload a .zip, .tar, .tar.gz or .tgz archive")
	}

	var total int64
	for _, e := range entries {
		raw := strings.TrimPrefix(strings.ReplaceAll(e.name, "\\", "/"), "./")
		escapes := path.IsAbs(raw)
		for _, seg := range strings.Split(raw, "/") {
			if seg == ".." {
				escapes = true
			}
		}
		if escapes {
			notes = append(notes, e.name+": a path outside the folder, left out")
			continue
		}
		name := path.Clean(raw)
		if junk(name) {
			continue
		}
		if e.size > maxFileBytes {
			notes = append(notes, fmt.Sprintf("%s: over %d KiB, left out", name, maxFileBytes>>10))
			continue
		}
		if total += e.size; total > maxUnpacked {
			return nil, nil, fmt.Errorf("the archive unpacks to over %d MiB", maxUnpacked>>20)
		}
		b, err := e.read()
		if err != nil {
			return nil, nil, fmt.Errorf("reading %s: %v", name, err)
		}
		if len(b) > maxFileBytes {
			notes = append(notes, fmt.Sprintf("%s: over %d KiB, left out", name, maxFileBytes>>10))
			continue
		}
		if !utf8.Valid(b) || bytes.IndexByte(b, 0) >= 0 {
			notes = append(notes, name+": not a text file, left out")
			continue
		}
		files = append(files, File{Path: name, Content: string(b)})
	}
	return stripCommonRoot(files), notes, nil
}

// ExtractArchiveRaw is ExtractArchive without the Ansible-specific
// clean-up: every text file of the archive, its single top folder stripped
// — for other kinds of projects (the Terraform panel's) to clean their way.
func ExtractArchiveRaw(filename string, data []byte) ([]File, []string, error) {
	return extract(filename, data)
}

// CleanUpload keeps the files of an upload that can be part of a playbook:
// it drops editor and VCS clutter, and leaves out (with a note) names a
// playbook path can't have and files the panel writes itself for each run —
// a project's own ansible.cfg and inventory are replaced by the panel's.
func CleanUpload(in []File) (files []File, notes []string) {
	seen := map[string]bool{}
	for _, f := range in {
		p := f.Path
		switch {
		case junk(p) || seen[p]:
			continue
		case reserved[p] || p == "inventory" || p == "hosts" || p == "hosts.ini" || p == "inventory.yml":
			notes = append(notes, p+": the panel builds the inventory and Ansible configuration for each run, so this was left out")
			continue
		case !pathRe.MatchString(p) || strings.Contains(p, "//"):
			notes = append(notes, p+": has characters a playbook path can't (letters, digits, . _ - @ + and /), left out")
			continue
		}
		seen[p] = true
		files = append(files, f)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, notes
}

// stripCommonRoot drops a single folder every file is in.
func stripCommonRoot(files []File) []File {
	if len(files) == 0 {
		return files
	}
	root, _, ok := strings.Cut(files[0].Path, "/")
	if !ok {
		return files
	}
	for _, f := range files {
		if !strings.HasPrefix(f.Path, root+"/") {
			return files
		}
	}
	for i := range files {
		files[i].Path = strings.TrimPrefix(files[i].Path, root+"/")
	}
	return files
}
