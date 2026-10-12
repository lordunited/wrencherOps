// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package deploy

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"devops-wrapper/internal/sshexec"
)

// SigNoz no longer ships Compose files (they're deprecated in its repo as of
// v0.144); it installs through Foundry: "foundryctl forge" turns a short
// casting.yaml into a generated Compose stack under pours/deployment, which
// is then started with plain "docker compose". The panel follows Foundry's
// documented step-by-step flow, so the running stack is an ordinary Compose
// project: Start, Stop, Down, Delete and Sync all work on it as on any other.

// FoundryVersion is the foundryctl release the panel installs: pinned, so a
// deploy is reproducible and its checksum is checked against that release.
const FoundryVersion = "v0.3.0"

// SigNozComposeDir is where forge writes the Compose stack, under the
// deploy folder.
const SigNozComposeDir = "pours/deployment"

// SigNozDefaultCasting is Foundry's own single-node Docker Compose example:
// SigNoz, ClickHouse, ClickHouse Keeper, Postgres and the OpenTelemetry
// ingester, with the UI on 8080 and OTLP on 4317/4318.
const SigNozDefaultCasting = `apiVersion: v1alpha1
kind: Installation
metadata:
  name: signoz
spec:
  deployment:
    flavor: compose
    mode: docker
`

// ValidateCasting checks a casting is one the panel can deploy — a Foundry
// Installation for Docker Compose — and returns its metadata.name, which
// Foundry uses as the Compose project name. Everything else in it is
// Foundry's to interpret, and forge reports its own errors.
func ValidateCasting(casting string) (name string, err error) {
	var c struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
		Metadata   struct {
			Name string `yaml:"name"`
		} `yaml:"metadata"`
		Spec struct {
			Deployment struct {
				Flavor string `yaml:"flavor"`
				Mode   string `yaml:"mode"`
			} `yaml:"deployment"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal([]byte(casting), &c); err != nil {
		return "", fmt.Errorf("casting.yaml isn't valid YAML: %v", err)
	}
	switch {
	case c.Kind != "" && c.Kind != "Installation":
		return "", fmt.Errorf("casting.yaml must be kind: Installation")
	case c.Metadata.Name == "":
		return "", fmt.Errorf("casting.yaml needs metadata.name")
	case c.Spec.Deployment.Flavor != "compose" || c.Spec.Deployment.Mode != "docker":
		return "", fmt.Errorf("the panel deploys SigNoz with Docker Compose: set spec.deployment to flavor: compose, mode: docker")
	}
	return c.Metadata.Name, nil
}

// foundryBin is where the pinned foundryctl is installed, inside the deploy
// folder: nothing is installed system-wide, and the folder's Delete removes
// it too.
func foundryBin(deployDir string) string {
	return deployDir + "/.foundry/foundryctl-" + FoundryVersion
}

// installFoundryScript downloads foundryctl for the server's architecture
// from SigNoz's GitHub release, checks it against the release's published
// SHA-256 checksums, and installs it — or does nothing if this version is
// already there.
func installFoundryScript(bin string) string {
	return fmt.Sprintf(`set -e
B=%s
V=%s
if [ -x "$B" ]; then echo "foundryctl $V is already installed."; exit 0; fi
case "$(uname -m)" in x86_64|amd64) A=amd64 ;; aarch64|arm64) A=arm64 ;; *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;; esac
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
U="https://github.com/SigNoz/foundry/releases/download/$V"
fetch() { if command -v curl >/dev/null 2>&1; then curl -fsSL -o "$2" "$1"; elif command -v wget >/dev/null 2>&1; then wget -q -O "$2" "$1"; else echo "The server needs curl or wget to download foundryctl." >&2; return 1; fi; }
echo "Downloading foundryctl $V (linux/$A)..."
fetch "$U/foundry_linux_$A.tar.gz" "$T/foundry.tgz"
fetch "$U/foundry_${V#v}_checksums.txt" "$T/checksums.txt"
want=$(grep " foundry_linux_$A.tar.gz\$" "$T/checksums.txt" | cut -d' ' -f1)
got=$(sha256sum "$T/foundry.tgz" | cut -d' ' -f1)
if [ -z "$want" ] || [ "$want" != "$got" ]; then echo "Checksum mismatch for foundry_linux_$A.tar.gz — not installing it." >&2; exit 1; fi
tar -xzf "$T/foundry.tgz" -C "$T"
mkdir -p "$(dirname "$B")"
cp "$T/foundry_linux_$A/bin/foundryctl" "$B" && chmod 755 "$B"
echo "Installed foundryctl $V (SHA-256 verified)."`, sshexec.ShellQuote(bin), FoundryVersion)
}

// configHashCmd prints a hash of every file forge generated, or "none"
// when nothing has been generated yet.
func configHashCmd(composeDir string) string {
	q := sshexec.ShellQuote(composeDir)
	script := fmt.Sprintf(`if [ -d %s ]; then cd %s && find . -type f -exec sha256sum {} + | sort | sha256sum | cut -d" " -f1; else echo none; fi`, q, q)
	return sshexec.AsRoot("sh -c " + sshexec.ShellQuote(script))
}

// RunSigNoz deploys SigNoz to deployDir: installs the pinned foundryctl if
// it isn't there, writes casting.yaml, forges the Compose stack, and starts
// it. When forging changed the generated configs of an existing install,
// the stack is recreated: Compose doesn't restart containers when only a
// mounted file changes, so they'd keep running the old config (Foundry's
// own upgrade note).
func (s *Store) RunSigNoz(target sshexec.Target, deployDir, casting string) *Job {
	job := s.create()
	composeDir := deployDir + "/" + SigNozComposeDir
	bin := foundryBin(deployDir)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				job.appendLog(fmt.Sprintf("panic: %v\n", r))
				job.setStatus(StatusFailed)
			}
		}()
		client := dial(job, target)
		if client == nil {
			return
		}
		defer client.Close()
		out := &logWriter{job: job}
		fail := func(msg string, err error) {
			job.appendLog(fmt.Sprintf("%s: %v\n", msg, err))
			job.setStatus(StatusFailed)
		}

		if !checkAvailable(job, client, out, sshexec.AsRoot("docker compose version"), "Docker Compose is available",
			"Install Docker with the Compose plugin: https://docs.docker.com/compose/install/") {
			return
		}

		job.appendLog("Checking foundryctl (SigNoz's installer)...\n")
		if err := client.Run(sshexec.AsRoot("sh -c "+sshexec.ShellQuote(installFoundryScript(bin))), out); err != nil {
			fail("Installing foundryctl failed", err)
			return
		}

		job.appendLog("Writing " + deployDir + "/casting.yaml...\n")
		if err := client.WriteFile(deployDir+"/casting.yaml", []byte(casting), out); err != nil {
			fail("Writing casting.yaml failed", err)
			return
		}

		before, _, _ := client.Output(configHashCmd(composeDir))
		job.appendLog("Generating the SigNoz stack (foundryctl forge)...\n")
		// cd as root: forge writes next to the casting, in a folder the SSH
		// user may not be allowed into.
		forge := sshexec.AsRootPrelude + "$AS_ROOT sh -c " + sshexec.ShellQuote(fmt.Sprintf("cd %s && %s forge -f casting.yaml --no-ledger --no-updater --format text",
			sshexec.ShellQuote(deployDir), sshexec.ShellQuote(bin)))
		if err := client.Run(forge, out); err != nil {
			fail("foundryctl forge failed", err)
			return
		}
		after, _, _ := client.Output(configHashCmd(composeDir))

		up := "up -d --remove-orphans"
		if b := strings.TrimSpace(before); b != "none" && b != "" && b != strings.TrimSpace(after) {
			job.appendLog("The generated configuration changed, so every container is recreated to pick it up.\n")
			up += " --force-recreate"
		}
		job.appendLog("Starting SigNoz (docker compose " + up + ")... the first start pulls several images and can take a few minutes.\n")
		start := sshexec.AsRootPrelude + sshexec.ComposeIn(composeDir) + " " + up
		if err := client.Run(start, out); err != nil {
			fail("Starting SigNoz failed", err)
			return
		}
		job.appendLog("SigNoz started. The UI is on port 8080 unless the casting changes it; ClickHouse takes a minute or two to be ready on first start.\n")
		job.setStatus(StatusSuccess)
	}()
	return job
}
