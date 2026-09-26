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
  - permission for that user to write to the install folder, which defaults to a folder under `/opt`

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

### Status and health

Each service shows two badges.

- **Status** is the result of the last action. It can be pending, deploying, deployed, starting, stopping, stopped, removing, down, or failed.
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
| Nexus3 is deployed but never becomes healthy | The Nexus data folder on the server must be writable by the Nexus container. Ask your server admin to fix the folder's ownership. |
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
