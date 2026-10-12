<div align="center">

# 🛠️ devops-wrapper

**One web panel for day-to-day DevOps work: GitLab, servers, Ansible, Terraform, and the tools you deploy.**

<p>
  <img alt="Go 1.27" src="https://img.shields.io/badge/Go-1.27-00ADD8?style=flat-square&logo=go&logoColor=white">
  <img alt="Next.js 16" src="https://img.shields.io/badge/Next.js-16-000000?style=flat-square&logo=nextdotjs&logoColor=white">
  <img alt="React 19" src="https://img.shields.io/badge/React-19-61DAFB?style=flat-square&logo=react&logoColor=black">
  <img alt="PostgreSQL 17" src="https://img.shields.io/badge/PostgreSQL-17-4169E1?style=flat-square&logo=postgresql&logoColor=white">
  <img alt="Docker Compose" src="https://img.shields.io/badge/Docker-Compose-2496ED?style=flat-square&logo=docker&logoColor=white">
  <img alt="OpenTofu" src="https://img.shields.io/badge/OpenTofu-Terraform-FFDA18?style=flat-square&logo=opentofu&logoColor=black">
  <img alt="Ansible" src="https://img.shields.io/badge/Ansible-core-EE0000?style=flat-square&logo=ansible&logoColor=white">
</p>

<p>
  <a href="#installation"><img alt="Get started" src="https://img.shields.io/badge/🚀_Get_started-2ea44f?style=for-the-badge"></a>
  <a href="#users-and-access"><img alt="Users and access" src="https://img.shields.io/badge/🔐_Access-d6336c?style=for-the-badge"></a>
  <a href="#troubleshooting"><img alt="Troubleshooting" src="https://img.shields.io/badge/🩺_Troubleshooting-6e7781?style=for-the-badge"></a>
  <a href="#tests"><img alt="Tests" src="https://img.shields.io/badge/✅_Tests-0969da?style=for-the-badge"></a>
</p>

</div>

---

## ✨ What it does

| | |
|---|---|
| <a href="#watching-gitlab"><img alt="GitLab" src="https://img.shields.io/badge/GitLab-e5484d?style=for-the-badge&logo=gitlab&logoColor=white"></a> | **Watch GitLab.** See projects, pipelines, merge requests, runners, and users, with a health page that flags what needs attention. |
| <a href="#watching-servers"><img alt="Servers" src="https://img.shields.io/badge/Servers-f2b705?style=for-the-badge&logo=linux&logoColor=black"></a> | **Watch your servers.** The panel logs in to each server over SSH every minute, reads its disks, memory, load, logins, kernel errors, and connections, and raises alerts from rules you control. |
| <a href="#ansible"><img alt="Ansible" src="https://img.shields.io/badge/Ansible-f05a28?style=for-the-badge&logo=ansible&logoColor=white"></a> | **Run Ansible playbooks** on those servers. Write them in the panel or upload a project, dry-run them, then apply them, with a full log of every run. |
| <a href="#terraform"><img alt="Terraform" src="https://img.shields.io/badge/Terraform-7c5cff?style=for-the-badge&logo=opentofu&logoColor=white"></a> | **Run Terraform** with OpenTofu. Keep workspaces in the panel, plan, review, and apply exactly the plan you reviewed. The panel keeps the state, encrypted, and checks the code against a built-in standard. |
| <a href="#installing-a-tool-on-a-server"><img alt="Deploy" src="https://img.shields.io/badge/Deploy-f59f00?style=for-the-badge&logo=docker&logoColor=white"></a> | **Install and manage tools on your servers.** Fill in a form and click Deploy to set up GitLab, Nexus3, a Prometheus and Grafana stack, Nginx, HAProxy, Envoy, SigNoz, or your own Compose project. You can also connect stacks you already run, then start, stop, check, reconfigure, or remove them from the same page. |
| <a href="#users-and-access"><img alt="Admin" src="https://img.shields.io/badge/Admin-d6336c?style=for-the-badge&logo=keycdn&logoColor=white"></a> | **Control who sees what.** Users, groups, and per-page access, checked by the backend on every request. |

Each badge matches the color of its section in the panel's sidebar. Click one to jump to that part of this guide.

---

## Before you start

You need:

- **A machine to run the panel**, with Docker and Docker Compose installed.
- **A Postgres database** for the panel. See [The panel's database](#the-panels-database) below before you start.
- **A GitLab access token** with the `read_api` scope. An admin token also shows runners and users for the whole GitLab instance.
- **For every server you want to manage:**
  - SSH login with a username and password. The panel doesn't use SSH keys.
  - the SSH user must be `root`, or be allowed to use `sudo` without a password. The panel needs this to create data folders with the right owner, edit root-owned config files, and read the system logs for server monitoring.
  - Docker with the Compose plugin, for anything you deploy from the **Deploy** section
  - `kubectl` with cluster access already set up, only if you use raw Kubernetes deploys

Ansible and OpenTofu don't need to be installed anywhere. The panel's backend image includes both.

---

## Installation

1. **Create your settings file.** In the project folder, copy the example:

   ```bash
   cp .env.example .env
   ```

2. **Add your GitLab details.** Open `.env` and set `GITLAB_BASE_URL` to your GitLab address. Then set `GITLAB_TOKEN` to your access token.

3. **Create a panel login.** Run this with the password you want:

   ```bash
   docker compose run --rm --no-deps --entrypoint /app/hashpassword backend 'your-password'
   ```

   Copy the output into `AUTH_USERS` as `username:output`, and keep the single quotes:

   ```
   AUTH_USERS='admin:$2a$10$....'
   ```

   To add more admins, separate them with commas inside the same quotes. Everyone else can be added later in the panel.

4. **Create an encryption key.** The panel uses it to encrypt saved server passwords, Terraform variables, Terraform state, and saved plans.

   ```bash
   docker compose run --rm --no-deps --entrypoint /app/genkey backend
   ```

   Paste the output into `CREDENTIAL_ENCRYPTION_KEY`.

   > [!CAUTION]
   > **Keep this key safe and never change it.** If it changes, every saved server password stops working and has to be entered again, and the panel can no longer read the Terraform state and variables it keeps.

5. **Check the database connection.** See [The panel's database](#the-panels-database).

6. **Start the panel.**

   ```bash
   docker compose up -d --build
   ```

   The first start takes a few minutes while the images build. The backend creates its database tables by itself on startup.

7. **Open the panel** at http://localhost:3000.

### The panel's database

The panel keeps everything in Postgres: users and sessions, services, servers, alerts, playbooks, runs, and Terraform workspaces and state.

> [!IMPORTANT]
> `docker-compose.yml` defines a `postgres` service, but **the backend is not connected to it right now.** The backend's `DATABASE_URL` points at `host.docker.internal:5432`, which is a Postgres running on the host machine, outside this compose project. `host.docker.internal` only works out of the box on Docker Desktop.

Pick one:

- **Use a Postgres on the host.** Make sure it's running on port 5432, with the user, password, and database name from `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB`. On Linux, also add this to the `backend` service in `docker-compose.yml`:

  ```yaml
  extra_hosts:
    - "host.docker.internal:host-gateway"
  ```

- **Use the bundled `postgres` service.** In `docker-compose.yml`, change the host in the backend's `DATABASE_URL` from `host.docker.internal` to `postgres`. Then add this to the `backend` service, so it waits for the database:

  ```yaml
  depends_on:
    postgres:
      condition: service_healthy
  ```

  The data is then kept in the `postgres-data` volume.

### Settings

These are the values in `.env` you may want to change:

| Setting | What it does | Default |
|---|---|---|
| `GITLAB_BASE_URL` | Address of your GitLab | `https://gitlab.com` |
| `GITLAB_TOKEN` | Your GitLab access token | none, required |
| `CACHE_TTL_SECONDS` | How often GitLab data is refreshed, in seconds | `30` |
| `AUTH_USERS` | The built-in admin logins | none, required |
| `SESSION_TTL_HOURS` | How long you stay signed in, in hours | `24` |
| `COOKIE_SECURE` | Set to `true` once the panel is served over HTTPS | `false` |
| `CREDENTIAL_ENCRYPTION_KEY` | Encrypts saved passwords and Terraform data | none, required |
| `POSTGRES_USER` | Database user name | `devops` |
| `POSTGRES_PASSWORD` | Database password | `devops` |
| `POSTGRES_DB` | Database name | `devops_wrapper` |
| `WEB_PORT` | Port the panel is served on | `3000` |

`DATABASE_URL` and `PORT` in `.env` are only used when you run the panel without Docker. Under Docker Compose, `docker-compose.yml` sets both itself.

After changing `.env`, apply it with `docker compose up -d`.

### Ports

| Port | What listens there |
|---|---|
| `3000` (`WEB_PORT`) | The panel. This is the address people open. |
| `8080` | The backend API. It's published on the host so a frontend started with `npm run dev` can reach it. |

> [!WARNING]
> Port `8080` is published on every network interface. If people other than you can reach this machine, change that line in `docker-compose.yml` to `"127.0.0.1:8080:8080"`, or remove it.

### Running without Docker

For development you can run each part on its own. You need Go 1.27, Node.js 22, a Postgres database, and, for the Ansible and Terraform pages, `ansible-playbook`, `sshpass`, and OpenTofu (`tofu`) on your `PATH`.

```bash
# Backend, on http://localhost:8080. It reads its settings from the environment.
set -a; source .env; set +a
go run ./cmd/server

# Frontend, on http://localhost:3000, in a second terminal
cd web
cp .env.local.example .env.local   # API_BASE_URL=http://localhost:8080
npm ci
npm run dev
```

---

## Signing in and out

Open the panel and sign in with a username and password from `AUTH_USERS`, or with a user an admin created in the panel. To sign out, click **Log out** in the top-right corner. You are signed out automatically when your session expires, which is after 24 hours by default.

Click your name in the top-right corner to see what you can access and to change your password.

---

## Users and access

The accounts in `AUTH_USERS` are built-in admins. They can't be changed or deleted from the panel, so there is always an admin who can sign in.

Admins add more users and groups in **Admin › Users & access**. Each user has a role:

| Role | What it can do |
|---|---|
| **admin** | Everything, in every tab, including managing users and access |
| **writer** | Exactly the access it's given, per tab: view or write |
| **viewer** | Look only, never change anything, whatever it's given |

Access is set per tab. Every page in the sidebar is listed, and each is set to **No access**, **View**, or **Write**. You can give access to a user directly, or to a group the user belongs to. A user gets the highest level from either, capped by their role. The GitLab pages, the Ansible module catalog, and the Ansible and Terraform run histories are view only for everyone. The Overview page is open to every signed-in user.

The backend checks access on every request, so hiding a page is not the only protection:

- Users only see the tabs they have access to.
- A page without access shows "You don't have access".
- With view access, buttons that change things are hidden. If such a request is sent anyway, it's refused.

Changes to a user's access apply on their next click. Disabling or deleting a user signs them out immediately. Changing a password signs out the user's other sessions.

> [!WARNING]
> **Write access to Ansible or Terraform is powerful.** A playbook can run commands on the panel's own backend as well as on your servers, and it receives the SSH passwords of the servers it runs on. Terraform code can run commands on the backend too. Give these tabs only to people you'd trust with your servers' passwords.

---

## Watching GitLab

The **GitLab** section of the sidebar has these pages:

| Page | What you see |
|---|---|
| **Health** | Things that need attention: runners that went offline or came back, projects whose last 2 or more pipelines failed, and merge requests open for 14 days or longer |
| **Projects** | All projects. Click one to see its recent pipelines, open merge requests, and members. |
| **Runners** | All runners and whether each is online |
| **Pipelines** | The most recent pipelines of every project |
| **Merge requests** | Open merge requests across all projects |
| **Users** | GitLab users |

Data refreshes about every 30 seconds. Reload the page to see the latest.

---

## Watching servers

**Servers › Servers** lists every server the panel watches. Servers that run a service deployed or connected from the **Deploy** section are added by themselves. To add another one, click **+ Add server** and enter its name, address, SSH port, user, and password. The panel logs in once to check the password before saving it, encrypted. A password the server refuses is never saved.

Saved passwords are shared by login: every service and server that uses the same address, port, and user uses the same saved password, so you enter it once.

Every 60 seconds the panel logs in to each server and reads:

- disks and inodes, per filesystem
- memory, swap, and CPU load
- who is logged in, recent successful SSH logins, and failed logins with the users and addresses they came from
- error lines in the kernel log
- open TCP connections, and the addresses with the most of them

Click a server to see all of it. Click **Refresh now** on a server, or **Refresh all** on the list, to read it again right away.

### Alert rules and notifications

**Servers › Notifications** shows the alerts that are firing and lets you acknowledge them. It also holds the rules that raise them. These rules come built in:

| Rule | Fires when | Severity |
|---|---|---|
| Disk almost full | a filesystem is 95% full | critical |
| Disk filling up | a filesystem is 85% full | warning |
| Inodes almost used up | a filesystem has used 95% of its inodes | critical |
| Many failed SSH logins | 20 or more failed logins in the last hour | warning |
| Too many open connections | 1000 or more established TCP connections | warning |
| Kernel errors | any error in the kernel log in the last hour | warning |
| High CPU load | the 5-minute load is 2 or more per CPU | warning |
| Memory almost full | 95% of memory is in use | warning |
| Heavy swapping | 80% of swap is in use (off by default) | warning |
| Server unreachable | the panel can't log in to the server | critical |

You can change a built-in rule's threshold and severity, or switch it off, but not delete it. You can add your own rules, for every server or for one.

---

## Ansible

### Playbooks

**Ansible › Playbooks** keeps your playbooks in the panel. To add one:

- **+ New playbook** starts from a template, and you edit it in the panel's editor.
- **Upload playbook or archive** takes a single playbook, or a `.zip`, `.tar`, `.tar.gz`, or `.tgz` of an Ansible project, up to 10 MB. **Upload folder** takes a project folder from your computer. The panel picks the file to run: `playbook.yml`, else `site.yml`, else `main.yml`, else the only playbook at the top level. If it can't tell, it asks you.

A playbook can have up to 200 text files, 1 MB each and 8 MB in total, in any folder layout: roles, group vars, templates, and so on.

**Ansible › Modules** is a catalog of Ansible modules with their options. From the editor you can insert a module as a ready-to-fill task.

### Running a playbook

Open a playbook, pick the servers to run it on, add extra vars if you need them (YAML or JSON), and choose a mode:

- **Dry run (--check --diff)** shows what would change and changes nothing.
- **Apply to N servers** runs it for real, with `--diff`.

**Apply needs a passing dry run first**, of the same saved playbook, the same extra vars, and the same servers. If you change any of them, run the dry run again. To apply without one, click **Apply without a dry run…** and type the playbook's name to confirm.

Every run checks the playbook's syntax first. The run log updates live, ends with the result for each server, and is kept in **Ansible › Run history**. You can cancel a run while it's going. A run is stopped after 1 hour.

The panel logs in with each server's saved password, and uses it for `become` too. The passwords are never written to disk, and they are masked in the run log. The panel doesn't check SSH host keys when it runs a playbook.

---

## Terraform

The Terraform pages run your code with [OpenTofu](https://opentofu.org), the open-source fork of Terraform. It uses the same `.tf` language and the same providers.

### Workspaces

**Terraform › Workspaces** holds one workspace per configuration. To create one, click **+ New workspace** and start from **Blank workspace** (versions, variables, main, and outputs files) or **Try it: a no-account example**, which needs no cloud account. You can also upload existing code. A workspace has these tabs:

- **Code**: the files, with a **Format** button that runs `tofu fmt`
- **Variables**: each one is either a Terraform variable (passed as `TF_VAR_<name>`) or an environment variable, such as a cloud provider's credentials. Mark secret ones **sensitive**. All values are stored encrypted, and sensitive ones are masked in run logs.
- **State**: the resources and outputs in the current state, and its earlier versions. Sensitive outputs are hidden.
- **Standards**: how the code measures up to the [Terraform standard](#terraform-standard)
- **Runs**: the plans and applies of this workspace

The panel keeps the state, encrypted, and saves a new version after every apply. If your code declares its own backend (for example `s3` or `pg`), OpenTofu uses that backend instead, and the panel doesn't keep a copy.

Providers are downloaded once and kept in the `tofu-plugins` volume, so later runs start faster.

### Plan, then apply

1. Click **Plan**, or **Plan destroy** to see what removing everything would do. Each plan runs `tofu init`, checks the formatting and the [standard](#terraform-standard), runs `tofu validate`, and then makes the plan. Nothing is changed yet.
2. Review the plan: what it creates, changes, replaces, destroys, or imports, and the full log.
3. Click **Apply** to apply **exactly that plan**.

An apply is refused if:

- the plan is over a day old
- the code changed since the plan was made
- the state changed since the plan was made, because another apply ran
- the plan was already applied, or has no changes

A plan that destroys or replaces anything needs the workspace's name typed before it can be applied.

Only one run goes at a time in each workspace. You can cancel a run. OpenTofu is interrupted first, so an apply stops cleanly and the state it reached is saved. A run is stopped after 2 hours. If an apply fails halfway, whatever it did change is still saved in the state.

Plans also have these options:

- **Replace:** plans chosen resources for replacement. This is what `terraform taint` became.
- **Import:** adds an `import` block, so the next plan and apply bring an existing object under management.

A workspace that still has resources in its state can't be deleted until you destroy them, or type its name to delete it anyway. The resources then stay, untracked.

### Terraform standard

Every Terraform workspace is checked against the panel's standard, which follows 30 Terraform concepts. You can see the result in a workspace's **Standards** tab. The code is checked as you edit and again before every plan.

These rules are **enforced**: if your code breaks one, it isn't planned.

| Rule | What it requires |
|---|---|
| TF001 | Every provider is in `required_providers` with a source and a version constraint |
| TF002 | `required_version` is set |
| TF003, TF004 | Every variable has a `type` and a `description` |
| TF005 | Secret-like variables (password, token, key…) are `sensitive = true` and have no default |
| TF006 | No secret is written in the code as a literal value |
| TF007, TF008 | Every output has a `description`, and secret-like outputs are `sensitive` |
| TF009 | Remote modules are pinned: `version` for registry modules, `?ref=` for Git |
| TF010 | The code is formatted with `tofu fmt` (click **Format** in the code tab) |
| TF011 | No `local` backend (the panel keeps the state) |
| TF012 | Every variable without a default has a value in the workspace's variables |
| TF013 | `count` and `for_each` are not used together |

These are **advice** only: provisioners (TF014), `depends_on` (TF015), and `ignore_changes = all` (TF016).

---

## Installing a tool on a server

The **Deploy** section has a page for each kind of tool:

| Page | What it sets up |
|---|---|
| **GitLab** | GitLab CE in Docker |
| **Nexus3** | Sonatype Nexus Repository, optionally with a Docker registry port |
| **Prometheus stack** | Prometheus, Grafana, Alertmanager, and Node Exporter |
| **Nginx** | Nginx as a reverse proxy or load balancer |
| **HAProxy** | HAProxy as a load balancer |
| **Envoy** | Envoy proxy |
| **SigNoz** | SigNoz observability (traces, metrics, and logs), installed with SigNoz's Foundry tool |
| **Custom compose** | Your own `docker-compose.yml`, on one or more servers |

There are also raw **Docker** and **Kubernetes** deploys, at `/deploy/docker` and `/deploy/kubernetes`. They aren't in the sidebar. They apply a Compose file or a Kubernetes manifest exactly as you give it. Kubernetes manifests are applied with `kubectl` on the server, using the cluster access already set up there.

To install GitLab, Nexus3, or a Prometheus stack:

1. Open its page in **Deploy**.
2. Click **+ New**, for example **+ New GitLab service**, and fill in the form.

   **Server details**, the same for every tool:

   | Field | Meaning |
   |---|---|
   | Name | A label for this service in the panel. It defaults to the host. |
   | Host | The server's IP address or hostname |
   | SSH port | Usually `22` |
   | Username | The SSH user on that server |
   | Deploy directory | The folder on the server where the tool and its data are stored |

   You are not asked for the SSH password here. You enter it when you deploy, unless the panel already has one saved for that login.

   **Tool options** depend on the tool:

   - **GitLab** asks for the external URL people will use to open GitLab, the initial `root` password with at least 8 characters, the version tag, and the web, HTTPS, and Git SSH ports.
   - **Nexus3** asks for the web port, an optional extra port for a Docker registry, and the version tag. It also has an optional Java memory size.
   - **Prometheus stack** asks for the Prometheus, Grafana, and Alertmanager ports, the Grafana admin password with at least 8 characters, how long to keep metrics, and the version tag.

   Under **Advanced** you can set CPU and memory limits, extra Docker networks, and extra folders to mount. GitLab also accepts extra GitLab settings, such as email or LDAP.

3. Click **Create service**. The service page opens and the service shows as **pending**.
4. Review the generated configuration. You can edit it here, then click **Save**.
5. Click **Deploy**, enter the server's SSH password if asked, and click **Confirm deploy**.
6. Watch the live log. The first deploy can take several minutes while the server downloads the images. When it finishes, the status changes to **deployed**.

The other tools work the same way: **+ New**, fill in the form, then **Deploy**. Their pages are described below.

The panel remembers the SSH password and stores it encrypted. Later actions don't ask for it again. Enter a new password in any password box to replace the saved one.

---

## Managing a service

Open a service from its list in the **Deploy** section. The buttons at the top control it:

| Button | What it does | Your data |
|---|---|---|
| **Sync** | Checks whether the service is running right now and updates its health. If something crashed, it shows a short reason. | Not touched |
| **Start** | Starts the service again after Stop or Down | Kept |
| **Stop** | Stops the service without removing anything | Kept |
| **Down** | Removes the running containers | Kept |
| **Delete** | Removes the service and **all of its data** from the server, then removes it from the panel | **Deleted permanently** |

> [!CAUTION]
> **Delete cannot be undone.** The panel asks you to confirm first. If the service was never deployed, Delete only removes it from the panel.

To change a service's settings, edit the fields or the configuration on its page, click **Save**, then click **Redeploy**.

A service page also shows each container's state and resource use, and every port the stack publishes.

### Activity history

At the bottom of every service page, **Activity history** lists everything the panel has done on that server for the service. That includes each Deploy, Start, Stop, Down, Delete, Sync, and config change, with the time, the result, and how long it took. The newest entries are at the top.

- Click an entry to read its full output.
- Click **Download log** to save that output as a text file.

The history is kept after you reload the page or restart the panel. If the panel restarts during an action, that entry is marked **failed** with a note saying it was interrupted.

### Connecting a stack you already run

To manage a Prometheus, Nginx, HAProxy, Envoy, or GitLab that was set up by hand, open its page in **Deploy** and click **Connect existing**.

1. Enter the server's address, SSH port, user, and password, and click **Find** (for example **Find Prometheus**). Leave the password empty if the panel already has one saved for that login.
2. The panel lists every matching container on that server. For each one it shows the compose project and file, and where its config lives. It doesn't matter how the compose file is named or where the config is mounted from.
3. Pick one, give it a name, and click **Connect**.

A connected stack gets the same config pages as one the panel deployed. A connected Prometheus, for example, opens on its **Rules & targets** tab.

The panel never deploys or deletes a connected stack, and its compose file is shown as a read-only copy. **Remove from panel** removes it from the panel and leaves the server untouched.

**Start, Stop and Down unlock after 5 healthy minutes.** The buttons are visible from the start but greyed out. After you connect, the panel checks the whole stack every 30 seconds by itself: every container must be running and passing its health check, and the tool itself must answer. For Prometheus, that means it's ready and running its last config. For Nginx, the config on disk must also pass `nginx -t`. A card on the page counts up to 5:00. Any failed check restarts the count. Once the stack has been healthy for 5 minutes in a row, the buttons unlock and stay unlocked, and an **Unlock** entry appears in the Activity history. They act on the stack's own Compose project and files, whatever those are called. **Down** keeps named volumes and data.

The SSH user needs access to Docker. To change config files owned by root, it must be `root` or be allowed to use `sudo` without a password.

### Prometheus: Rules & targets

A Prometheus service page has a second tab, **Rules & targets**. It connects to the server and shows what the running Prometheus has actually loaded:

- how many targets are up and down, and which alerts are firing
- every scrape job, with each target's health and its last error
- every alert rule and whether it is firing, pending, or quiet
- the config file and rule files themselves

From there you can:

- **Add targets** to an existing job. Type one per line, and add a note after `#`, for example `172.22.241.34:9100 # vpn node exporter`.
- **Create a scrape job.** Pick Exporter, Website check, Port check, or SSL certificate. Blackbox jobs pre-fill your blackbox exporter address from your existing jobs.
- **Add an alert rule** from a template, such as Target down, High CPU, Disk almost full, Website down, or SSL certificate expiring. The PromQL editor completes metric and label names from your Prometheus.
- **Edit a file** directly.

Every change shows you a before and after of each file first. When you click **Apply to server**, the panel checks the change with Prometheus's own checker, saves a backup of each file under `/var/backups/devops-panel/` on the server, writes the change, and reloads Prometheus without restarting it. If Prometheus rejects the change, nothing is written or the old files are put back. Each change is recorded in the Activity history.

If the page says the config lives **inside the container only**, click **Move config to disk** first. Otherwise your changes would be lost on the next redeploy. Prometheus restarts once, with the same configuration it was running.

Search finds jobs, targets, and rules, including by the comment next to a target in your config, such as `# vpn node exporter`. If Prometheus is protected with TLS or a password through a web config file, the panel says so and can't read it yet.

### Prometheus: Status

The **Status** tab shows what's happening inside Prometheus, read live from the server:

- **Needs attention** at the top: failed config reloads, storage or WAL corruption, failing writes, a disk getting full, failed compactions or checkpoints, failing rules, dropped alerts, or no Alertmanager connected.
- **TSDB and head block**: series, chunks, the time range held in memory, stored blocks, retention and compactions.
- **Write-ahead log (WAL)**: size and segments on disk, the current segment, startup replay progress, average fsync time, truncations, checkpoints and corruptions.
- **Runtime**: uptime, memory, goroutines, file handles and the last config reload.
- **Rules and alerting**: evaluations and failures, connected Alertmanagers, and notifications sent or dropped.
- **Where the series come from**: the metrics, labels and label values that make up most of the TSDB.
- **Build** details and every **startup flag**.

Click **Refresh** to read it again.

Both **Rules & targets** and **Status** open instantly with the last reading the panel saved, marked with its time, and bring in a fresh reading behind it. Changes unlock once the fresh reading has arrived. If the server can't be reached, the saved reading stays on screen with a note saying why.

### Nginx configuration

**Deploy → Nginx** manages Nginx configuration: server blocks, upstreams and config files. It doesn't manage website content.

- **+ New Nginx** runs the official Nginx image with its config in a `conf.d` folder on the server, and a `certs` folder mounted read-only at `/etc/nginx/certs`. It starts with a default server that answers `/healthz` and refuses everything else.
- **Connect existing** finds the Nginx containers on a server, including nginx-unprivileged and OpenResty, however their compose files and mounts are set up.

The **Nginx config** tab shows the configuration as Nginx itself loads it:

- the Nginx version, whether the config on disk passes `nginx -t`, CPU, memory and process count, and recent errors from its log
- every **server block** with its domains, listen ports, HTTPS certificate and what each location does, searchable by domain, port, path or target
- every **upstream** with its backends and balancing method
- the main **global settings**, such as worker processes, `client_max_body_size`, gzip and includes
- every **config file**, with **Edit** and **Remove**

**+ New server block** writes a reverse proxy, a load balancer (with its own upstream, round robin, least connections or sticky by client), or a redirect as its own file. It can add HTTPS with your certificate, send HTTP visitors to HTTPS, allow WebSockets, and set an upload limit and a timeout.

Every change shows a before and after first. **Apply to server** saves a backup of each file under `/var/backups/devops-panel/`, writes the change and runs `nginx -t`. Only if the test passes does Nginx reload, gracefully, without dropping open connections. If the test fails, the previous files are put back and Nginx keeps serving as before. Every change is recorded in the Activity history.

### HAProxy configuration

**Deploy → HAProxy** works like the Nginx page, for HAProxy's config format.

- **+ New HAProxy** runs the official HAProxy image with its config on the server. You choose the HTTP and HTTPS ports, the image tag, and the same **Advanced** options as the other tools.
- **Connect existing** finds the HAProxy containers on a server. For a connected HAProxy, the panel works out how to check its health from the config it runs: its `monitor-uri`, else its stats page, else whether its ports accept connections.

The **HAProxy config** tab shows every frontend and backend, the live health of each backend server, recent errors from the HAProxy log, and the config files. **+ New site** adds a frontend and its backend for you. If there's no stats page, **Enable stats page** adds one, so the panel can show live backend health.

Every change shows a before and after first. **Test configuration (haproxy -c)** checks it without changing anything, and **Apply to server** stays locked until that test passes. Applying saves a backup, writes the change, and reloads HAProxy gracefully, without dropping connections. If HAProxy rejects it, the previous files are put back.

The **Kubernetes** tab of this page isn't built yet.

### Envoy configuration

**Deploy → Envoy** manages Envoy's bootstrap config.

- **+ New Envoy** runs the official `envoyproxy/envoy` image with its config on the server.
- **Connect existing** finds the Envoy containers on a server.

The **Envoy config** tab shows what Envoy reports through its admin API: whether it's ready, its listeners and routes, and each cluster with the health of every endpoint. **+ Add a route** writes a route and its cluster for you from a form. **Edit envoy.yaml** edits the whole file. Every change shows a before and after first, and **Test (validate only)** checks it with `envoy --mode validate` without changing anything.

Envoy can't reload a static config, so **Apply** restarts Envoy, and only after the change has passed validation. If Envoy doesn't come back ready, the previous file is put back and Envoy is restarted again.

### SigNoz

**Deploy → SigNoz** installs [SigNoz](https://signoz.io) the way SigNoz now recommends, with its Foundry tool. The panel installs a pinned version of Foundry, checks its checksum, generates the SigNoz Compose stack with it, and starts it with `docker compose`. From then on it's an ordinary Compose project on the server: Start, Stop, Down, Delete, and Sync work on it as on any other service.

### Custom compose

**Deploy → Custom compose** runs your own `docker-compose.yml`.

- **+ New custom compose** creates an app from your compose file, its `.env` values, and any other files it needs, such as configs it mounts. You can deploy one app to several servers. The panel keeps one copy and pushes every change to each server, so they can't drift apart.
- **Detect on a server** logs in to a server and lists the Compose projects already running there. For each one you can:
  - **Import as app**. The panel then manages it, and you can deploy it to more servers.
  - **Watch** it as it is. The panel monitors it and never changes it.
- **Env files**: from Detect you can open a project's `.env` and every `env_file` its services load, and edit them in place. Each save is checked against the version you opened, backed up first, and can be applied with `docker compose up -d`. That recreates only the containers whose environment changed.

### Status and health

Each service shows two badges.

- **Status** is the result of the last action. It can be pending, deploying, deployed, starting, stopping, stopped, removing, down, or failed. A connected stack always shows **connected**.
- **Health** is the result of the last **Sync**:
  - **healthy** means everything is running
  - **degraded** means something is not running or is failing, and the reason is shown next to it
  - **unreachable** means the panel could not reach the server or found nothing running
  - **unknown** means the service has not been synced yet

---

## After installing

- **GitLab:** open the external URL and sign in as `root` with the password you set. The first start can take 5 minutes or more.
- **Nexus3:** open `http://<server>:<web port>` and sign in as `admin`. The first password is in the `admin.password` file inside the `nexus-data` folder in the deploy directory on the server.
- **Grafana:** open `http://<server>:<Grafana port>` and sign in as `admin` with the password you set. Prometheus is already connected as a data source.

---

## Troubleshooting

| Problem | What to check |
|---|---|
| The backend keeps restarting with `database:` in its log | It can't reach Postgres. See [The panel's database](#the-panels-database). |
| Can't sign in | Make sure `AUTH_USERS` in `.env` is wrapped in single quotes. Then run `docker compose up -d`. |
| GitLab pages show an error | Check `GITLAB_BASE_URL` and `GITLAB_TOKEN`, and make sure the token has not expired |
| Deploy stops at "Connecting" | Check the host, SSH port, username, and password |
| "Docker Compose is not available" | Install Docker with the Compose plugin on the target server |
| A container fails with "permission denied" | Click **Start** or **Redeploy**. Both fix the owner of the Nexus3, Prometheus, Grafana, and Alertmanager data folders before starting. |
| "Could not prepare data folders" in the log | The SSH user is not `root` and can't use `sudo` without a password. Use `root`, or allow that user passwordless `sudo`. |
| A status stays at deploying or stopping after the panel restarted | Click **Sync**, or run the action again |
| Prometheus stack fails to start with a custom version tag | All four images share one tag. Use `latest`, or a tag that exists for all of them. |
| A server shows no failed logins or kernel errors | The SSH user can't read the system logs. Use `root`, or allow passwordless `sudo`. |
| Apply in Ansible says to run a dry run first | Run a dry run with the same files, extra vars, and servers, and wait for it to pass |
| A Terraform plan stops at formatting (TF010) | Click **Format** in the code tab, save, and plan again |
| A Terraform apply says the code or state changed | Plan again, then apply the new plan |
| `tofu init` can't download a provider | The backend must be able to reach the OpenTofu registry and the provider's download site |

To see the panel's own logs:

```bash
docker compose logs -f backend web
```

---

## Tests

```bash
go test ./...            # backend unit tests
cd web && npm test       # frontend unit tests
cd web && npm run lint   # frontend lint
```

The end-to-end tests run only when `TEST_DATABASE_URL` is set. Point it at a throwaway Postgres, never at real data. The Terraform end-to-end test also needs `tofu` installed.

---

## Updating and stopping the panel

- **Update after pulling new code:** `docker compose up -d --build`
- **Stop the panel:** `docker compose down`
- **Start it again:** `docker compose up -d`

Everything the panel knows, including services, servers, logins, saved passwords, playbooks, and Terraform state, lives in its Postgres database. Back that database up together with `CREDENTIAL_ENCRYPTION_KEY`; one is no use without the other. Stopping or updating the panel never touches the servers you deployed to.

## Tips

> [!TIP]
> New to the panel? Start with **Try it: a no-account example** in **Terraform › Workspaces**, and a **Dry run** in **Ansible**. Neither changes anything you care about.

- Serve the panel over HTTPS behind a reverse proxy, and then set `COOKIE_SECURE=true`. The panel doesn't limit repeated login attempts itself, so let the reverse proxy rate-limit `/api/auth/login`.
- Don't leave the backend's port `8080` open to your network. See [Ports](#ports).
- Give each person their own account in **Admin › Users & access**, with only the tabs they need. Keep the `AUTH_USERS` admins for yourself.
- Change the default database password (`devops`) before you put the panel anywhere others can reach it.

---

**Author:** Mohammadreza Masoumi — DevOps Engineer · <s.masoumimohammadreza@gmail.com>

Copyright © 2026 Mohammadreza Masoumi.
