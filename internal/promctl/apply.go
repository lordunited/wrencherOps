package promctl

import (
	"encoding/base64"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"devops-wrapper/internal/sshexec"
)

// Change is a new version of one file, identified by its path inside the
// container. BaseSHA is the hash of the version the change was made from,
// or "" for a file that doesn't exist yet; Apply refuses to overwrite a
// file that has changed on the server since.
type Change struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	BaseSHA string `json:"baseSha"`
}

// BackupRoot is where the previous version of every changed file is kept,
// mirroring the file's own path. Backups live outside the config and rule
// folders on purpose: a rule_files glob like "rules/*" would otherwise
// load a backup as a second copy of the rules, and fire every alert twice.
const BackupRoot = "/var/backups/devops-panel"

// BackupPath is where the backup of path taken at stamp is kept.
func BackupPath(p, stamp string) string {
	return BackupRoot + path.Clean("/"+p) + "." + stamp
}

// asRoot runs the rest of a shell command as root: directly when the SSH
// user is root, otherwise through passwordless sudo.
const asRoot = `if [ "$(id -u)" -eq 0 ]; then AS_ROOT=""; else AS_ROOT="sudo -n"; fi; `

// Apply validates changes with promtool, backs up and writes each file, and
// hot-reloads Prometheus, confirming the reload took. Progress goes to log.
// If the reload fails, the backups are restored and Prometheus is reloaded
// again, so a failed apply leaves the old configuration running.
func Apply(c *sshexec.Client, inst *Instance, changes []Change, log io.Writer) error {
	if len(changes) == 0 {
		return fmt.Errorf("nothing to apply")
	}
	for _, ch := range changes {
		if !path.IsAbs(ch.Path) {
			return fmt.Errorf("%s is not an absolute path", ch.Path)
		}
	}

	fmt.Fprintln(log, "Checking that the files haven't changed on the server since they were read...")
	if err := checkBase(c, inst, changes); err != nil {
		return err
	}

	fmt.Fprintln(log, "Validating with promtool...")
	if out, err := validate(c, inst, changes); err != nil {
		fmt.Fprintln(log, strings.TrimSpace(out))
		return fmt.Errorf("promtool rejected the change, so nothing was written")
	} else {
		fmt.Fprintln(log, indent(strings.TrimSpace(out)))
	}

	var before struct {
		LastConfigTime string `json:"lastConfigTime"`
	}
	if err := api(c, inst, "status/runtimeinfo", &before); err != nil {
		return err
	}

	stamp := time.Now().UTC().Format("20060102-150405")
	var written []Change
	for _, ch := range changes {
		where, err := write(c, inst, ch, stamp)
		if err != nil {
			restore(c, inst, written, stamp, log)
			return fmt.Errorf("writing %s: %v", ch.Path, err)
		}
		written = append(written, ch)
		fmt.Fprintf(log, "Wrote %s\n", where)
	}

	fmt.Fprintln(log, "Reloading Prometheus...")
	if err := reload(c, inst, before.LastConfigTime); err != nil {
		fmt.Fprintf(log, "Reload failed: %v\nRestoring the previous files...\n", err)
		restore(c, inst, written, stamp, log)
		if rerr := reload(c, inst, ""); rerr != nil {
			fmt.Fprintf(log, "Reloading the previous files also failed: %v\n", rerr)
		}
		return fmt.Errorf("Prometheus did not accept the new configuration; the previous files were restored")
	}
	fmt.Fprintln(log, "Prometheus reloaded the new configuration.")
	return nil
}

func indent(s string) string {
	if s == "" {
		return ""
	}
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func exec(inst *Instance, root bool, script string, args ...string) string {
	cmd := "docker exec "
	if root {
		cmd += "-u 0 "
	}
	cmd += sshexec.ShellQuote(inst.Container) + " sh -c " + sshexec.ShellQuote(script) + " sh"
	for _, a := range args {
		cmd += " " + sshexec.ShellQuote(a)
	}
	return cmd
}

func checkBase(c *sshexec.Client, inst *Instance, changes []Change) error {
	var paths []string
	for _, ch := range changes {
		paths = append(paths, ch.Path)
	}
	current, err := readFiles(c, inst, paths)
	if err != nil {
		return err
	}
	now := map[string]string{}
	for _, f := range current {
		now[f.Path] = f.SHA
	}
	for _, ch := range changes {
		got, exists := now[ch.Path]
		switch {
		case ch.BaseSHA == "" && exists:
			return fmt.Errorf("%s was created on the server in the meantime; inspect again and retry", ch.Path)
		case ch.BaseSHA != "" && !exists:
			return fmt.Errorf("%s no longer exists on the server; inspect again and retry", ch.Path)
		case ch.BaseSHA != "" && got != ch.BaseSHA:
			return fmt.Errorf("%s was changed on the server since it was read; inspect again and retry", ch.Path)
		}
	}
	return nil
}

// validate copies the config folder to a scratch folder inside the
// container, puts the changed files in place there, and runs promtool
// against the copy. Relative rule_files paths resolve the same way they
// will for the real config. Changed files outside the config folder are
// rule files and are checked on their own.
func validate(c *sshexec.Client, inst *Instance, changes []Change) (string, error) {
	dir := inst.ConfigDir()
	scratch := "/tmp/panel-check"
	script := strings.Builder{}
	script.WriteString(`set -e; S="$1"; D="$2"; rm -rf "$S"; mkdir -p "$S"; cp -r "$D"/. "$S"/; shift 2; `)
	var args []string
	args = append(args, scratch, dir)
	var standalone []string
	for i, ch := range changes {
		if rel, ok := strings.CutPrefix(ch.Path, dir+"/"); ok {
			args = append(args, rel, b64(ch.Content))
			script.WriteString(`mkdir -p "$S/$(dirname "$1")"; echo "$2" | base64 -d > "$S/$1"; shift 2; `)
			// A config that lists its rules by absolute path would check
			// the old copy on disk, so check each changed rule file itself.
			if ch.Path != inst.ConfigPath {
				standalone = append(standalone, scratch+"/"+rel)
			}
		} else {
			tmp := fmt.Sprintf("/tmp/panel-check-rule-%d.yml", i)
			args = append(args, tmp, b64(ch.Content))
			script.WriteString(`echo "$2" | base64 -d > "$1"; shift 2; `)
			standalone = append(standalone, tmp)
		}
	}
	script.WriteString(`promtool check config "$S/` + path.Base(inst.ConfigPath) + `"; `)
	for _, f := range standalone {
		script.WriteString(`promtool check rules ` + sshexec.ShellQuote(f) + `; `)
	}
	// On failure set -e stops before this cleanup; the next validation
	// starts by clearing the scratch folder anyway.
	script.WriteString(`rm -rf "$S" /tmp/panel-check-rule-*.yml`)

	stdout, stderr, err := c.Output(exec(inst, false, script.String(), args...))
	return strings.TrimSpace(stdout + "\n" + stderr), err
}

// write backs up and replaces one file. Files on a bind mount are written
// on the server, in place, so a container that mounts the single file
// (which pins its inode) still sees the new content. Files that only live
// in the container are written there.
func write(c *sshexec.Client, inst *Instance, ch Change, stamp string) (string, error) {
	if host, ok := inst.HostPath(ch.Path); ok {
		backup := BackupPath(host, stamp)
		cmd := asRoot + fmt.Sprintf(
			`F=%s; B=%s; $AS_ROOT mkdir -p "$(dirname "$F")" && { [ ! -f "$F" ] || { $AS_ROOT mkdir -p "$(dirname "$B")" && $AS_ROOT cp -p "$F" "$B"; }; } && echo %s | base64 -d | $AS_ROOT tee "$F" > /dev/null`,
			sshexec.ShellQuote(host), sshexec.ShellQuote(backup), b64(ch.Content))
		if _, stderr, err := c.Output(cmd); err != nil {
			return "", fmt.Errorf("%v %s", err, strings.TrimSpace(stderr))
		}
		return host + " on the server (previous version kept at " + backup + ")", nil
	}
	backup := BackupPath(ch.Path, stamp)
	script := `F="$1"; B="$2"; mkdir -p "$(dirname "$F")"; [ ! -f "$F" ] || { mkdir -p "$(dirname "$B")" && cp -p "$F" "$B"; }; echo "$3" | base64 -d > "$F"`
	if _, stderr, err := c.Output(exec(inst, true, script, ch.Path, backup, b64(ch.Content))); err != nil {
		return "", fmt.Errorf("%v %s", err, strings.TrimSpace(stderr))
	}
	return ch.Path + " inside the container (previous version kept at " + backup + " in the container)", nil
}

// restore puts back the backups written by write, and deletes files that
// were newly created.
func restore(c *sshexec.Client, inst *Instance, written []Change, stamp string, log io.Writer) {
	for _, ch := range written {
		var err error
		if host, ok := inst.HostPath(ch.Path); ok {
			cmd := asRoot + fmt.Sprintf(`F=%s; B=%s; if [ -f "$B" ]; then $AS_ROOT cat "$B" | $AS_ROOT tee "$F" > /dev/null; else $AS_ROOT rm -f "$F"; fi`,
				sshexec.ShellQuote(host), sshexec.ShellQuote(BackupPath(host, stamp)))
			_, _, err = c.Output(cmd)
		} else {
			script := `F="$1"; B="$2"; if [ -f "$B" ]; then cat "$B" > "$F"; else rm -f "$F"; fi`
			_, _, err = c.Output(exec(inst, true, script, ch.Path, BackupPath(ch.Path, stamp)))
		}
		if err != nil {
			fmt.Fprintf(log, "Could not restore %s: %v\n", ch.Path, err)
		} else {
			fmt.Fprintf(log, "Restored %s\n", ch.Path)
		}
	}
}

// reload sends SIGHUP and waits for Prometheus to report a successful
// reload newer than previous (or any successful reload when previous is
// empty).
func reload(c *sshexec.Client, inst *Instance, previous string) error {
	if _, stderr, err := c.Output("docker kill -s HUP " + sshexec.ShellQuote(inst.Container)); err != nil {
		return fmt.Errorf("sending the reload signal failed: %v %s", err, strings.TrimSpace(stderr))
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(time.Second)
		var info struct {
			ReloadConfigSuccess bool   `json:"reloadConfigSuccess"`
			LastConfigTime      string `json:"lastConfigTime"`
		}
		if err := api(c, inst, "status/runtimeinfo", &info); err != nil {
			continue
		}
		if !info.ReloadConfigSuccess {
			return fmt.Errorf("Prometheus reports the reload failed; see the Prometheus container logs")
		}
		if previous == "" || info.LastConfigTime != previous {
			return nil
		}
	}
	return fmt.Errorf("Prometheus did not confirm the reload within 15 seconds")
}
