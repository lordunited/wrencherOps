// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"devops-wrapper/internal/deploy"
	"devops-wrapper/internal/promctl"
	"devops-wrapper/internal/sshexec"
)

// Detecting compose projects already on a server, to import one as an app
// (the panel then manages it, and can deploy it to more servers) or to watch
// one as it is (connected: monitored, never changed).

// DetectedProject is a compose project found on a server.
type DetectedProject struct {
	Name        string   `json:"name"`
	WorkingDir  string   `json:"workingDir"`
	ConfigFiles []string `json:"configFiles"`
	// Status is what "docker compose ls" says, e.g. "running(2)" or
	// "exited(1)"; empty for a compose file found on disk that Docker has
	// no containers for.
	Status string `json:"status,omitempty"`

	Importable    bool   `json:"importable"`
	ImportProblem string `json:"importProblem,omitempty"`
	Watchable     bool   `json:"watchable"`
	WatchProblem  string `json:"watchProblem,omitempty"`

	// The panel's own record of it, when there is one.
	ServiceID   string `json:"serviceId,omitempty"`
	ServiceName string `json:"serviceName,omitempty"`
	ServiceType string `json:"serviceType,omitempty"`
	AppID       string `json:"appId,omitempty"`

	// EnvFiles are the env files its compose uses: .env, and every env_file.
	EnvFiles []EnvFile `json:"envFiles,omitempty"`

	// otherComposeFiles are compose files in the folder that the project
	// doesn't use.
	otherComposeFiles []string
}

// otherComposeFiles reports whether the project's folder holds compose
// files besides the one it runs from: then which file a plain "docker
// compose" reads there depends on name precedence, not on the project.
func otherComposeFiles(p *DetectedProject) bool {
	return len(p.otherComposeFiles) > 0
}

// scanRoots are where compose files are looked for on disk, a few levels
// deep — the usual homes of hand-made stacks. Running projects are found
// wherever they are, from Docker itself.
var scanRoots = []string{"/opt", "/srv", "/home", "/root", "/var/www", "/docker", "/data", "/apps"}

// scanProjects lists the compose projects on the server: every project
// Docker has containers for ("docker compose ls --all", wherever its files
// are), then compose files under scanRoots that none of those use.
func (h *Handler) scanProjects(r *http.Request, client *sshexec.Client, host string) ([]*DetectedProject, error) {
	out, errOut, err := client.Output(sshexec.AsRoot("docker compose ls --all --format json"))
	if err != nil {
		return nil, fmt.Errorf("docker compose ls failed (the SSH user needs Docker with the Compose plugin, directly or via sudo): %v %s", err, strings.TrimSpace(errOut))
	}
	var listed []struct{ Name, Status, ConfigFiles string }
	if strings.TrimSpace(out) != "" {
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &listed); err != nil {
			return nil, fmt.Errorf("reading docker compose ls: %v", err)
		}
	}

	var projects []*DetectedProject
	known := map[string]bool{}
	for _, l := range listed {
		p := &DetectedProject{Name: l.Name, Status: l.Status}
		for _, f := range strings.Split(l.ConfigFiles, ",") {
			if f = strings.TrimSpace(f); f != "" {
				p.ConfigFiles = append(p.ConfigFiles, f)
				known[f] = true
			}
		}
		if len(p.ConfigFiles) > 0 {
			p.WorkingDir = path.Dir(p.ConfigFiles[0])
		}
		projects = append(projects, p)
	}

	var find strings.Builder
	find.WriteString("find")
	for _, root := range scanRoots {
		find.WriteString(" " + sshexec.ShellQuote(root))
	}
	find.WriteString(` -maxdepth 4 -type f \(`)
	for i, n := range deploy.ComposeFileNames {
		if i > 0 {
			find.WriteString(" -o")
		}
		find.WriteString(" -name " + n)
	}
	find.WriteString(` \) -not -path '*/node_modules/*' -not -path '*/.git/*' 2>/dev/null; true`)
	found, _, _ := client.Output(sshexec.AsRoot(find.String()))
	// A compose file next to a project Docker already knows belongs to that
	// project's folder (it changes which file compose would pick there), not
	// to a separate project in the same folder.
	byDir := map[string]*DetectedProject{}
	for _, p := range projects {
		byDir[p.WorkingDir] = p
	}
	onDisk := map[string][]string{}
	for _, f := range strings.Split(strings.TrimSpace(found), "\n") {
		if f = strings.TrimSpace(f); f == "" || known[f] {
			continue
		}
		if p, ok := byDir[path.Dir(f)]; ok {
			p.otherComposeFiles = append(p.otherComposeFiles, f)
			continue
		}
		onDisk[path.Dir(f)] = append(onDisk[path.Dir(f)], f)
	}
	for dir, files := range onDisk {
		sort.Strings(files)
		projects = append(projects, &DetectedProject{Name: deploy.DefaultProjectName(dir), WorkingDir: dir, ConfigFiles: files})
	}
	sort.Slice(projects, func(a, b int) bool { return projects[a].WorkingDir < projects[b].WorkingDir })

	services, err := h.services.List(r.Context())
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		for _, s := range services {
			if s.Host != host {
				continue
			}
			sameDir := strings.TrimRight(s.DeployDir, "/") == p.WorkingDir
			if sameDir || (s.Connected() && s.ComposeProject == p.Name) {
				p.ServiceID, p.ServiceName, p.ServiceType, p.AppID = s.ID, s.Name, s.Type, s.AppID
				break
			}
		}
		judge(p)
		p.EnvFiles = envFilesOf(client, p)
	}
	return projects, nil
}

// judge decides what can safely be done with a detected project. Import
// hands it to the panel, which then deploys the same compose file, under the
// same name, in the same folder, under compose's default project name; a
// project set up any other way (several -f files, a file compose wouldn't
// find by itself, a -p name) would end up with a second copy started beside
// it, so it can only be watched.
func judge(p *DetectedProject) {
	switch {
	case p.AppID != "":
		p.ImportProblem = "already deployed by a custom compose app"
	case p.ServiceID != "":
		p.ImportProblem = fmt.Sprintf("already in the panel as %q", p.ServiceName)
	case len(p.ConfigFiles) != 1 && p.Status == "":
		p.ImportProblem = "its folder has more than one compose file, so which one compose reads depends on the names; keep one to import it"
	case len(p.ConfigFiles) != 1:
		p.ImportProblem = "it uses several compose files together; the panel deploys a single one"
	case !deploy.IsComposeFileName(path.Base(p.ConfigFiles[0])):
		p.ImportProblem = fmt.Sprintf("its compose file is %s, a name docker compose doesn't find by itself (it's started with -f); rename it on the server to one of %s to import it", path.Base(p.ConfigFiles[0]), strings.Join(deploy.ComposeFileNames, ", "))
	case otherComposeFiles(p):
		p.ImportProblem = "its folder has more than one compose file, so which one compose reads depends on the names; keep one to import it"
	case p.Name != deploy.DefaultProjectName(p.WorkingDir):
		p.ImportProblem = fmt.Sprintf("it runs as project %q, not the folder's default %q, so redeploying it from the panel would start a second copy beside it", p.Name, deploy.DefaultProjectName(p.WorkingDir))
	default:
		p.Importable = true
	}
	switch {
	case p.ServiceID != "":
		p.WatchProblem = "the panel already watches or manages it"
	case p.Status == "":
		p.WatchProblem = "it has no containers to watch; import it, or start it on the server first"
	default:
		p.Watchable = true
	}
}

func (h *Handler) handleDetectProjects(w http.ResponseWriter, r *http.Request) {
	var req connectTarget
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	client, err := h.dialTarget(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer client.Close()
	projects, err := h.scanProjects(r, client, req.Host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if projects == nil {
		projects = []*DetectedProject{}
	}
	writeJSON(w, projects, nil)
}

type projectRequest struct {
	connectTarget
	WorkingDir string `json:"workingDir"`
	Name       string `json:"name"`
}

// findProject re-scans the server and returns the project in dir, so what
// was judged when the list was shown is judged again before acting.
func (h *Handler) findProject(r *http.Request, client *sshexec.Client, host, dir string) (*DetectedProject, error) {
	projects, err := h.scanProjects(r, client, host)
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		if p.WorkingDir == dir {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no compose project in %s any more; scan again", dir)
}

type importResponse struct {
	App   *App     `json:"app"`
	Notes []string `json:"notes"`
}

// handleImportProject turns a project on a server into a custom compose
// app with that server as its first: its docker-compose.yml, its .env, and
// the files its compose mounts or reads (env_file) — text files in the
// folder, up to the project limits. Folders it mounts stay on that server
// only, and are listed in the notes. Nothing on the server is changed.
func (h *Handler) handleImportProject(w http.ResponseWriter, r *http.Request) {
	var req projectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	client, err := h.dialTarget(r.Context(), &req.connectTarget)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer client.Close()
	p, err := h.findProject(r, client, req.Host, req.WorkingDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	app, notes, err := h.importProject(r.Context(), client, req.connectTarget, p, req.Name)
	if err != nil {
		var he *httpError
		if errors.As(err, &he) {
			http.Error(w, he.msg, he.code)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, importResponse{App: app, Notes: notes}, nil)
}

// httpError is an error with the HTTP status it should be reported with.
type httpError struct {
	code int
	msg  string
}

func (e *httpError) Error() string { return e.msg }

// importProject makes project p, found on the server client is connected
// to, a custom compose app with that server as its first.
func (h *Handler) importProject(ctx context.Context, client *sshexec.Client, t connectTarget, p *DetectedProject, name string) (*App, []string, error) {
	if !p.Importable {
		return nil, nil, &httpError{http.StatusConflict, "can't import this project: " + p.ImportProblem}
	}

	dir := p.WorkingDir
	readText := func(rel string) (content string, note string) {
		full := dir + "/" + rel
		info, _, err := client.Output(sshexec.AsRoot("stat -c '%F|%s' " + sshexec.ShellQuote(full)))
		if err != nil {
			return "", rel + " doesn't exist on the server"
		}
		kind, size, _ := strings.Cut(strings.TrimSpace(info), "|")
		if kind == "directory" {
			return "", rel + " is a folder: it stays on this server only — add its files to the project to deploy it elsewhere"
		}
		if n, _ := strconv.Atoi(size); n > deploy.MaxProjectFileBytes {
			return "", rel + " is over 1 MiB, so it wasn't imported"
		}
		out, errOut, err := client.Output(sshexec.AsRoot("cat " + sshexec.ShellQuote(full)))
		if err != nil {
			return "", fmt.Sprintf("couldn't read %s: %v %s", rel, err, strings.TrimSpace(errOut))
		}
		return out, ""
	}

	composeFile := path.Base(p.ConfigFiles[0])
	manifest, note := readText(composeFile)
	if note != "" {
		return nil, nil, &httpError{http.StatusBadGateway, note}
	}
	var notes []string
	env := ""
	if _, _, err := client.Output(sshexec.AsRoot("test -f " + sshexec.ShellQuote(dir+"/.env"))); err == nil {
		if env, note = readText(".env"); note != "" {
			notes = append(notes, note)
		}
	}

	analysis := deploy.AnalyzeProject(manifest, env, nil, dir)
	if analysis.ParseError != "" {
		return nil, nil, &httpError{http.StatusBadRequest, "the compose file isn't valid YAML: " + analysis.ParseError}
	}
	var candidates []string
	for _, m := range analysis.Mounts {
		if m.Kind == "bind" && !strings.HasPrefix(m.Source, "/") && !strings.HasPrefix(m.Source, "~") {
			candidates = append(candidates, m.Source)
		}
	}
	candidates = append(candidates, analysis.EnvFiles...)
	seen := map[string]bool{".env": true, composeFile: true}
	var files []deploy.ProjectFile
	for _, c := range candidates {
		rel := path.Clean(c)
		if seen[rel] || rel == "." {
			continue
		}
		seen[rel] = true
		if rel == ".." || strings.HasPrefix(rel, "../") {
			notes = append(notes, c+" is outside the folder, so it stays on this server only")
			continue
		}
		if err := deploy.ValidProjectPath(rel); err != nil {
			notes = append(notes, err.Error())
			continue
		}
		content, note := readText(rel)
		if note != "" {
			notes = append(notes, note)
			continue
		}
		f := deploy.ProjectFile{Path: rel, Content: content}
		if err := deploy.ValidateProjectFiles([]deploy.ProjectFile{f}); err != nil {
			notes = append(notes, err.Error())
			continue
		}
		files = append(files, f)
	}
	if err := validateFiles(files); err != nil {
		return nil, nil, &httpError{http.StatusBadRequest, err.Error()}
	}

	name = strings.TrimSpace(name)
	if name == "" {
		name = p.Name
	}
	app := &App{Name: name, Manifest: manifest, ComposeFile: composeFile, Env: env, Files: files, DeployDir: dir}
	if err := h.apps.Create(ctx, app); err != nil {
		return nil, nil, err
	}
	svc, err := h.addServer(ctx, app, appServer{Host: t.Host, Port: t.Port, Username: t.Username, Password: t.Password})
	if err != nil {
		return nil, nil, fmt.Errorf("the app was created, but adding this server failed: %w", err)
	}
	// It's running already, as this exact project: record it as deployed
	// rather than pending, so it isn't shown as never deployed.
	if p.Status != "" {
		if updated, err := h.services.Update(ctx, svc.ID, func(s *Service) { s.Status = StatusDeployed }); err == nil {
			svc = updated
		}
	}
	app.Servers = []*Service{svc}
	app.analyze()
	if notes == nil {
		notes = []string{}
	}
	return app, notes, nil
}

// handleWatchProject connects a project as it is: the panel monitors its
// containers (Sync and the health monitor), and its Start/Stop/Down unlock
// after five healthy minutes like any connected stack — but it's never
// deployed, edited or removed by the panel.
func (h *Handler) handleWatchProject(w http.ResponseWriter, r *http.Request) {
	var req projectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	client, err := h.dialTarget(r.Context(), &req.connectTarget)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer client.Close()
	p, err := h.findProject(r, client, req.Host, req.WorkingDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	svc, err := h.watchProject(r.Context(), client, req.connectTarget, p, req.Name)
	if err != nil {
		var he *httpError
		if errors.As(err, &he) {
			http.Error(w, he.msg, he.code)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, svc, nil)
}

// watchProject connects project p, found on the server client is connected
// to, as it is.
func (h *Handler) watchProject(ctx context.Context, client *sshexec.Client, t connectTarget, p *DetectedProject, name string) (*Service, error) {
	if !p.Watchable {
		return nil, &httpError{http.StatusConflict, "can't watch this project: " + p.WatchProblem}
	}
	encrypted, err := h.secrets.Encrypt(t.Password)
	if err != nil {
		return nil, err
	}
	manifest := promctl.ReadHostFiles(client, p.ConfigFiles)
	if manifest == "" {
		manifest = "# The compose file(s) could not be read: " + strings.Join(p.ConfigFiles, ", ") + "\n"
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = p.Name
	}
	svc := &Service{
		Name: name, Type: "docker", Host: t.Host, Port: t.Port, Username: t.Username,
		DeployDir: p.WorkingDir, Manifest: manifest, Status: StatusConnected, EncryptedPassword: encrypted,
		Origin: OriginConnected, ComposeProject: p.Name, ComposeFiles: strings.Join(p.ConfigFiles, ","),
	}
	started := time.Now()
	if err := h.services.Create(ctx, svc); err != nil {
		return nil, err
	}
	svc.HasSavedPassword = true
	h.recordAction(ctx, svc, "connect", "success",
		fmt.Sprintf("Watching compose project %s on %s (%s)\nCompose file(s): %s\nThe panel never deploys, edits or removes it. Start, Stop and Down unlock once it has been healthy for 5 minutes in a row.\n",
			p.Name, t.Host, p.WorkingDir, strings.Join(p.ConfigFiles, ", ")), started)
	return svc, nil
}

// AddedProject is what scanning did with one project it added.
type AddedProject struct {
	WorkingDir string   `json:"workingDir"`
	Name       string   `json:"name"`
	As         string   `json:"as"` // "app" or "watched"
	Href       string   `json:"href,omitempty"`
	Notes      []string `json:"notes,omitempty"`
	Error      string   `json:"error,omitempty"`
}

type scanAddResponse struct {
	Added    []AddedProject     `json:"added"`
	Projects []*DetectedProject `json:"projects"`
}

// handleScanAndAdd scans a server and adds every compose project it runs —
// every one with containers — that isn't in the panel yet: as an app when
// it can be imported, else watched as it is. Compose files with no
// containers are only listed. Nothing on the server is changed. The list
// returned is a fresh scan, so it shows what's in the panel now.
func (h *Handler) handleScanAndAdd(w http.ResponseWriter, r *http.Request) {
	var req connectTarget
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	client, err := h.dialTarget(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer client.Close()
	projects, err := h.scanProjects(r, client, req.Host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	added := []AddedProject{}
	for _, p := range projects {
		if p.Status == "" || p.AppID != "" || p.ServiceID != "" {
			continue
		}
		a := AddedProject{WorkingDir: p.WorkingDir, Name: p.Name}
		switch {
		case p.Importable:
			a.As = "app"
			app, notes, err := h.importProject(r.Context(), client, req, p, "")
			if err != nil {
				a.Error = err.Error()
			} else {
				a.Href, a.Notes = "/deploy/compose/"+app.ID, notes
			}
		case p.Watchable:
			a.As = "watched"
			svc, err := h.watchProject(r.Context(), client, req, p, "")
			if err != nil {
				a.Error = err.Error()
			} else {
				a.Href = "/deploy/compose/watch/" + svc.ID
			}
		default:
			a.Error = "can't be added: " + p.ImportProblem + "; " + p.WatchProblem
		}
		added = append(added, a)
	}

	if len(added) > 0 {
		if fresh, err := h.scanProjects(r, client, req.Host); err == nil {
			projects = fresh
		}
	}
	if projects == nil {
		projects = []*DetectedProject{}
	}
	writeJSON(w, scanAddResponse{Added: added, Projects: projects}, nil)
}
