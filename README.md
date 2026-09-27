# devops-wrapper

devops-wrapper is a web panel for your DevOps work. It does two things:

- **Keeps an eye on your GitLab.** You can see projects, pipelines, merge requests, runners, and users in one place. It also flags what needs attention.
- **Installs and manages tools on your own servers.** You fill in a form and click Deploy, and the panel sets up GitLab, Nexus3, or a Prometheus and Grafana monitoring stack on the server you choose. After that you can start, stop, check, or remove it from the same page.

---

## Before you start

You need:

- **A machine to run the panel**, with Docker and Docker Compose installed.
- **A GitLab access token** with the `read_api` scope. An admin token also shows runners and users for the whole GitLab instance.
- **For every server you want to install tools on:**
  - SSH login with a username and password
  - Docker with the Compose plugin installed
  - the SSH user must be `root`, or be allowed to use `sudo` without a password. The panel needs this to create each tool's data folders with the right owner.

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

   To add more people, separate them with commas inside the same quotes.

4. **Create an encryption key.** The panel uses it to protect saved server passwords.

   ```bash
   docker compose run --rm --no-deps --entrypoint /app/genkey backend
   ```

   Paste the output into `CREDENTIAL_ENCRYPTION_KEY`.

   > **Keep this key safe and never change it.** If it changes, every saved server password stops working and has to be entered again.

5. **Start the panel.**

   ```bash
   docker compose up -d --build
   ```

   The first start takes a few minutes while the images build.

6. **Open the panel** at http://localhost:3000.

### Settings

These are the values in `.env` you may want to change:

| Setting | What it does | Default |
|---|---|---|
| `GITLAB_BASE_URL` | Address of your GitLab | `https://gitlab.com` |
| `GITLAB_TOKEN` | Your GitLab access token | none, required |
| `CACHE_TTL_SECONDS` | How often GitLab data is refreshed, in seconds | `30` |
| `AUTH_USERS` | Panel logins | none, required |
| `SESSION_TTL_HOURS` | How long you stay signed in, in hours | `24` |
| `COOKIE_SECURE` | Set to `true` once the panel is served over HTTPS | `false` |
| `CREDENTIAL_ENCRYPTION_KEY` | Protects saved server passwords | none, required |
| `POSTGRES_PASSWORD` | Password for the panel's internal database | `devops` |
| `WEB_PORT` | Port the panel is served on | `3000` |

After changing `.env`, apply it with `docker compose up -d`.

---

## Signing in and out

Open the panel and sign in with a username and password from `AUTH_USERS`. To sign out, click **Log out** in the top-right corner, next to your username. You are signed out automatically when your session expires, which is after 24 hours by default.

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

## Installing a tool on a server

1. In the **Deploy** section of the sidebar, choose **GitLab**, **Nexus3**, or **Prometheus stack**.
2. Click **+ New** and fill in the form.

   **Server details**, the same for every tool:

   | Field | Meaning |
   |---|---|
   | Name | A label for this service in the panel. It defaults to the host. |
   | Host | The server's IP address or hostname |
   | SSH port | Usually `22` |
   | Username | The SSH user on that server |
   | Deploy directory | The folder on the server where the tool and its data are stored |

   You are not asked for the SSH password here. You enter it when you deploy.

   **Tool options** depend on the tool:

   - **GitLab** asks for the external URL people will use to open GitLab, the initial `root` password with at least 8 characters, the version tag, and the web, HTTPS, and Git SSH ports.
   - **Nexus3** asks for the web port, an optional extra port for a Docker registry, and the version tag. It also has an optional Java memory size.
   - **Prometheus stack** asks for the Prometheus, Grafana, and Alertmanager ports, the Grafana admin password with at least 8 characters, how long to keep metrics, and the version tag.

   Under **Advanced** you can set CPU and memory limits, extra Docker networks, and extra folders to mount. GitLab also accepts extra GitLab settings, such as email or LDAP.

3. Click **Create service**. The service page opens and the service shows as **pending**.
4. Review the generated configuration. You can edit it here, then click **Save**.
5. Click **Deploy**, enter the server's SSH password, and click **Confirm deploy**.
6. Watch the live log. The first deploy can take several minutes while the server downloads the images. When it finishes, the status changes to **deployed**.

The panel remembers the SSH password for that service and stores it encrypted. Later actions don't ask for it again. Enter a new password in any password box to replace the saved one.

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

> **Delete cannot be undone.** The panel asks you to confirm first. If the service was never deployed, Delete only removes it from the panel.

To change a service's settings, edit the fields or the configuration on its page, click **Save**, then click **Redeploy**.

### Activity history

At the bottom of every service page, **Activity history** lists everything the panel has done on that server for the service. That includes each Deploy, Start, Stop, Down, Delete, and Sync, with the time, the result, and how long it took. The newest entries are at the top.

- Click an entry to read its full output.
- Click **Download log** to save that output as a text file.

The history is kept after you reload the page or restart the panel. If the panel restarts during an action, that entry is marked **failed** with a note saying it was interrupted.

### Prometheus: Rules & targets

A Prometheus service page has a second tab, **Rules & targets**. It connects to the server and shows what the running Prometheus has actually loaded:

- how many targets are up and down, and which alerts are firing
- every scrape job, with each target's health and its last error
- every alert rule and whether it is firing, pending, or quiet
- the config file and rule files themselves

From there you can:

- **Add targets** to an existing job. Type one per line, and add a note after `#`, for example `172.22.241.34:9100 # vpn node exporter`.
- **Create a scrape job.** Pick Exporter, Website check, Port check, or SSL certificate. Blackbox jobs pre-fill your blackbox exporter address from your existing jobs.
- **Add an alert rule** from a template, such as Target down, High CPU, Disk almost full, Website down, or SSL certificate expiring.
- **Edit a file** directly.

Every change shows you a before and after of each file first. When you click **Apply to server**, the panel checks the change with Prometheus's own checker, saves a backup of each file under `/var/backups/devops-panel/` on the server, writes the change, and reloads Prometheus without restarting it. If Prometheus rejects the change, nothing is written or the old files are put back. Each change is recorded in the Activity history.

If the page says the config lives **inside the container only**, click **Move config to disk** first. Otherwise your changes would be lost on the next redeploy. Prometheus restarts once, with the same configuration it was running.

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

### Connecting a Prometheus you already run

To manage a Prometheus that was set up by hand, go to **Deploy → Prometheus stack** and click **Connect existing**.

1. Enter the server's address, SSH port, user and password, and click **Find Prometheus**.
2. The panel lists every Prometheus container on that server. For each one it shows the compose project and file, where the config file lives, and the port and URL prefix. It doesn't matter how the compose file is named or where the config is mounted from.
3. Pick one, give it a name, and click **Connect**.

A connected Prometheus opens on its **Rules & targets** tab, with everything described above. Search finds jobs, targets and rules, including by the comment next to a target in your config, such as `# vpn node exporter`.

The panel never deploys or deletes a connected stack, and its compose file is shown as a read-only copy. **Remove from panel** removes it from the panel and leaves the server untouched.

**Start, Stop and Down unlock after 5 healthy minutes.** The buttons are visible from the start but greyed out. After you connect, the panel checks the whole stack every 30 seconds by itself: every container must be running and passing its health check, and Prometheus must be ready and running its last config. A card on the page counts up to 5:00. Any failed check restarts the count. Once the stack has been healthy for 5 minutes in a row, the buttons unlock and stay unlocked, and an **Unlock** entry appears in the Activity history. They act on the stack's own Compose project and files, whatever those are called. **Down** keeps named volumes and data.

The SSH user needs access to Docker. To change config files owned by root, it must be `root` or be allowed to use `sudo` without a password. If Prometheus is protected with TLS or a password through a web config file, the panel says so and can't read it yet.

### Nginx configuration

**Deploy → Nginx** manages Nginx configuration: server blocks, upstreams and config files. It doesn't manage website content.

- **+ New Nginx** runs the official Nginx image with its config in a `conf.d` folder on the server, and a `certs` folder mounted read-only at `/etc/nginx/certs`. It starts with a default server that answers `/healthz` and refuses everything else.
- **Connect existing** finds the Nginx containers on a server, including nginx-unprivileged and OpenResty, however their compose files and mounts are set up. Connected Nginx servers are protected the same way as connected Prometheus stacks.

The **Nginx config** tab shows the configuration as Nginx itself loads it:

- the Nginx version, whether the config on disk passes `nginx -t`, CPU, memory and process count, and recent errors from its log
- every **server block** with its domains, listen ports, HTTPS certificate and what each location does, searchable by domain, port, path or target
- every **upstream** with its backends and balancing method
- the main **global settings**, such as worker processes, `client_max_body_size`, gzip and includes
- every **config file**, with **Edit** and **Remove**

**+ New server block** writes a reverse proxy, a load balancer (with its own upstream, round robin, least connections or sticky by client), or a redirect as its own file. It can add HTTPS with your certificate, send HTTP visitors to HTTPS, allow WebSockets, and set an upload limit and a timeout.

Every change shows a before and after first. **Apply to server** saves a backup of each file under `/var/backups/devops-panel/`, writes the change and runs `nginx -t`. Only if the test passes does Nginx reload, gracefully, without dropping open connections. If the test fails, the previous files are put back and Nginx keeps serving as before. Every change is recorded in the Activity history.

For a connected Nginx, the 5-minute health check also requires the config on disk to pass `nginx -t`.

### Status and health

Each service shows two badges.

- **Status** is the result of the last action. It can be pending, deploying, deployed, starting, stopping, stopped, removing, down, or failed. A connected Prometheus always shows **connected**.
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
| Can't sign in | Make sure `AUTH_USERS` in `.env` is wrapped in single quotes. Then run `docker compose up -d`. |
| GitLab pages show an error | Check `GITLAB_BASE_URL` and `GITLAB_TOKEN`, and make sure the token has not expired |
| Deploy stops at "Connecting" | Check the host, SSH port, username, and password |
| "Docker Compose is not available" | Install Docker with the Compose plugin on the target server |
| A container fails with "permission denied" | Click **Start** or **Redeploy**. Both fix the owner of the Nexus3, Prometheus, Grafana, and Alertmanager data folders before starting. |
| "Could not prepare data folders" in the log | The SSH user is not `root` and can't use `sudo` without a password. Use `root`, or allow that user passwordless `sudo`. |
| A status stays at deploying or stopping after the panel restarted | Click **Sync**, or run the action again |
| Prometheus stack fails to start with a custom version tag | All four images share one tag. Use `latest`, or a tag that exists for all of them. |

To see the panel's own logs:

```bash
docker compose logs -f backend web
```

---

## Updating and stopping the panel

- **Update after pulling new code:** `docker compose up -d --build`
- **Stop the panel:** `docker compose down`
- **Start it again:** `docker compose up -d`

Your services, logins, and saved passwords are kept in the database volume. Stopping or updating the panel never touches the servers you deployed to.

## Tips

- Serve the panel over HTTPS behind a reverse proxy, and then set `COOKIE_SECURE=true`.
- Everyone who can sign in has full control over every service. Only give accounts to people who should manage your servers.
