// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package deploy

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"io/fs"
	"sort"
	"strings"
)

// SigNoz as a plain compose project. SigNoz no longer ships compose files of
// its own (its deploy/ folder now points to Foundry), so the template is what
// Foundry generates for the default casting, kept here: a compose.yaml and
// the config files it mounts, which become an ordinary custom compose app —
// every file visible and editable in the panel, deployable to any number of
// servers, with no Foundry on them.

// SigNozVersion and SigNozCollectorVersion are the images the template pins
// (through .env, so an upgrade is an edit there). The collector is the
// release current when this SigNoz was.
const (
	SigNozVersion          = "v0.144.0"
	SigNozCollectorVersion = "v0.144.12"
)

// SigNozTemplate names the template on the apps it creates.
const SigNozTemplate = "signoz"

//go:embed signozcompose
var signozCompose embed.FS

// signozAssets holds histogram-quantile v0.0.1 for linux/amd64 and arm64,
// exactly as released (SHA-256 checked against the release's checksums when
// added): ClickHouse's histogramQuantile function, which SigNoz's setup step
// would otherwise download from GitHub on the server.
//
//go:embed signozassets
var signozAssets embed.FS

// SigNozBundledFiles are the binaries a SigNoz deploy writes next to the
// compose, under histogram-quantile/. They're added at deploy time rather
// than kept with the app: they're binary, and nothing to edit.
func SigNozBundledFiles() []ProjectFile {
	entries, _ := signozAssets.ReadDir("signozassets")
	var files []ProjectFile
	for _, e := range entries {
		b, err := signozAssets.ReadFile("signozassets/" + e.Name())
		if err == nil {
			files = append(files, ProjectFile{Path: "histogram-quantile/" + e.Name(), Content: string(b)})
		}
	}
	return files
}

// ComposeTemplate is a ready-to-create custom compose app.
type ComposeTemplate struct {
	Template    string        `json:"template"`
	Name        string        `json:"name"`
	DeployDir   string        `json:"deployDir"`
	ComposeFile string        `json:"composeFile"`
	Manifest    string        `json:"manifest"`
	Env         string        `json:"env"`
	Files       []ProjectFile `json:"files"`
}

// SigNozComposeTemplate returns the SigNoz template with a fresh random
// Postgres password in its .env, so no two apps share one and none uses a
// published default.
func SigNozComposeTemplate() (ComposeTemplate, error) {
	t := ComposeTemplate{Template: SigNozTemplate, Name: "signoz", DeployDir: "/opt/signoz", ComposeFile: "compose.yaml"}
	err := fs.WalkDir(signozCompose, "signozcompose", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := signozCompose.ReadFile(p)
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(p, "signozcompose/")
		if rel == t.ComposeFile {
			t.Manifest = string(b)
		} else {
			t.Files = append(t.Files, ProjectFile{Path: rel, Content: string(b)})
		}
		return nil
	})
	if err != nil {
		return ComposeTemplate{}, err
	}
	sort.Slice(t.Files, func(i, j int) bool { return t.Files[i].Path < t.Files[j].Path })

	pw := make([]byte, 24)
	if _, err := rand.Read(pw); err != nil {
		return ComposeTemplate{}, err
	}
	t.Env = "# Image versions: change them and deploy to upgrade.\n" +
		"SIGNOZ_VERSION=" + SigNozVersion + "\n" +
		"OTELCOL_VERSION=" + SigNozCollectorVersion + "\n" +
		"\n" +
		"# SigNoz's own Postgres. Postgres only takes this when its volume is first\n" +
		"# created: changing it afterwards locks SigNoz out of its database.\n" +
		"SIGNOZ_POSTGRES_PASSWORD=" + hex.EncodeToString(pw) + "\n"
	return t, nil
}
