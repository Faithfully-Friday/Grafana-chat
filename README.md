# A2A Chat — Grafana App Plugin

An OpenWebUI-style chat interface inside Grafana for LLM agents that speak the
[A2A (Agent2Agent) protocol](https://a2aproject.github.io/A2A/) — including
LangGraph and `deepagents` deployments, which expose A2A endpoints natively.

- Streaming token-by-token replies (`message/stream` over SSE), with automatic
  fallback to blocking `message/send`
- Conversation memory on the agent side via A2A `contextId`
- Chat history persisted in an **embedded SQLite** database inside the plugin —
  no external database needed
- Responsive layout: sidebar with conversation list on desktop, drawer on mobile
- Markdown rendering (incl. GFM tables/code) in assistant replies

## Architecture

```
Grafana UI (React)
   │  /api/plugins/myorg-a2achat-app/resources/...
   ▼
Plugin Go backend ── embedded SQLite (chat history, contextId mapping)
   │  JSON-RPC: message/stream · message/send
   ▼
Your A2A agent (LangGraph deployment, a2a-sdk server, ...)
```

The agent endpoint URL and API key are stored in the plugin settings
(server-side), so credentials never reach the browser.

## Prerequisites

- Node.js ≥ 22 and npm
- Go ≥ 1.24 (for the backend)
- Docker (for the dev Grafana)
- An A2A-compatible agent endpoint

## Development

```bash
# 1. Install frontend dependencies
npm install

# 2. Build the frontend (or `npm run dev` to watch)
npm run build

# 3. Build the backend binaries
mage build          # all platforms
# or just the one you need:
GOOS=linux GOARCH=arm64 go build -o dist/gpx_a2a_chat_linux_arm64 ./pkg

# 4. Start a dev Grafana with the plugin mounted
docker compose up

# 5. Open http://localhost:3000 (admin/admin)
```

## Running unsigned (development only)

The steps above already work out of the box because the bundled
`docker-compose.yaml` tells Grafana to trust this specific plugin ID without a
signature — see `GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS: myorg-a2achat-app`
in `.config/docker-compose-base.yaml`. This section is for running unsigned
on a Grafana instance **other than** that bundled dev container — e.g. an
existing dev/test Grafana you already have running elsewhere.

Grafana refuses to load unsigned plugins unless explicitly told which plugin
IDs to trust. Set one of:

- **grafana.ini**:
  ```ini
  [plugins]
  allow_loading_unsigned_plugins = myorg-a2achat-app
  ```
- **Environment variable** (equivalent, e.g. for a Docker deployment):
  ```
  GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=myorg-a2achat-app
  ```

  Both take a comma-separated list, so multiple plugin IDs can be trusted at
  once if needed.

Then copy the built `dist/` into that Grafana's plugins directory as
`myorg-a2achat-app` (see the plugin directory paths in the [production
installation](#production-installation-self-hosted-behind-nginx) section
below) and restart Grafana.

**Never do this in production.** It disables Grafana's plugin signature
verification entirely for the listed plugin ID(s) — acceptable for a local
box only you control, a real problem on anything reachable by others. For an
actual deployment, see [Signing the plugin](#signing-the-plugin) below
instead.

## Signing the plugin

This plugin ships **unsigned**, which is fine for local dev (see above), but
a real Grafana instance rejects unsigned plugins by default. Getting a
**private signature** is one command once the prerequisites are in place —
the setup is what takes a few minutes, not the signing itself.

**One-time setup:**

1. Create a [Grafana Cloud](https://grafana.com) account (the free tier is
   enough — this is only used to issue a signing token, not to host anything).
2. In the Cloud Portal: **My Account → Security → Access Policies** → create
   a policy scoped to **`plugins:write`** → generate a token.
3. Export it in your shell (never commit it):
   ```bash
   export GRAFANA_ACCESS_POLICY_TOKEN=<your-token>
   ```

**Sign for a specific Grafana instance:**

```bash
npm run sign -- --rootUrls https://your-grafana-host/
```

A **private signature is cryptographically locked to `--rootUrls`**. It must
match the target instance's `root_url` (`GF_SERVER_ROOT_URL`) exactly —
scheme, host, port, and trailing slash all count. Signing for
`https://your-grafana-host/` produces a signature that is only valid on a
Grafana instance configured with that exact root URL; pointing that same
signed `dist/` at a different host shows right back up as unsigned/invalid.

A few things that make this cheap to get right over time:

- **Multiple targets at once**: `--rootUrls` takes a comma-separated list,
  e.g. `--rootUrls https://staging.example.com,https://grafana.example.com`,
  producing one signature valid for both.
- **Re-signing later is just re-running the same command** with the new/updated
  URL(s) — it regenerates `dist/MANIFEST.txt` in place. The same access-policy
  token works for repeat signs until it expires.
- **Re-sign after every rebuild.** `dist/` is what gets checksummed and
  signed — a fresh `npm run build` / backend rebuild invalidates the previous
  signature, so signing is the last step before deploying, not a one-time
  setup step.

> **Grafana Cloud (managed SaaS) cannot run this plugin at all** — signed or
> not. Per Grafana's own docs: *"You can only add plugins that are uploaded to
> the Grafana plugins catalog to your Grafana Cloud instance. Private,
> custom-built, or third-party plugins that require manual uploading or
> manually modifying Grafana backend files cannot be installed on or used
> with Grafana Cloud."* There's no signing flag or config workaround for
> this — a Cloud stack (`*.grafana.net`) is simply not a valid deployment
> target for a custom plugin like this one. It only runs on Grafana OSS/
> Enterprise instances you host yourself.

## Production installation (self-hosted, behind nginx)

Worked example for deploying a signed build behind an nginx reverse proxy at
the domain root (e.g. `https://your-grafana-host/`) — distinct from the repo's
own dev `docker-compose.yaml`, which runs as root and force-loads unsigned
plugins and should never be reused as-is for this.

**1. Build**

```bash
npm ci
npm run build
GOOS=linux GOARCH=<arch> go build -o dist/gpx_a2a_chat_linux_<arch> ./pkg
chmod +x dist/gpx_a2a_chat_linux_<arch>
```

Match `<arch>` (`amd64`/`arm64`) to the Docker host's CPU — check with
`docker exec <container> uname -m` if unsure. If cutting a real release,
bump `"version"` in `package.json` first; the build tooling copies it into
`plugin.json`.

**2. Sign it for this instance**

```bash
npm run sign -- --rootUrls https://your-grafana-host/
```

**3. Grafana configuration** (env vars on the Grafana container/service):

| Variable | Value | Why |
| --- | --- | --- |
| `GF_SERVER_ROOT_URL` | `https://your-grafana-host/` | Must match `--rootUrls` exactly |
| `GF_PLUGIN_DATA_DIR` | `/var/lib/grafana/plugin-data/myorg-a2achat-app` | Where the plugin's SQLite chat history lives (see below) |

> **Persisting chat history across redeploys**: `pkg/plugin/store.go` opens
> its SQLite DB under `GF_PLUGIN_DATA_DIR` (falling back to a relative
> `data/` directory otherwise, which only survives by accident in the dev
> setup). Point it at a path under Grafana's own persistent data volume (the
> same one that already holds `grafana.db`) so it's already writable by the
> `grafana` user and survives container recreation without a separate
> volume.

**4. nginx configuration** (TLS termination + reverse proxy at the domain
root — adjust `ssl_certificate`/`ssl_certificate_key` for your setup):

```nginx
server {
    listen 443 ssl;
    server_name your-grafana-host;

    ssl_certificate     /etc/nginx/ssl/fullchain.pem;
    ssl_certificate_key /etc/nginx/ssl/privkey.pem;

    location / {
        proxy_set_header Host $host;
        proxy_pass http://grafana:3000;
    }

    # Grafana Live (WebSocket) needs the upgrade headers explicitly —
    # nginx does not forward these by default.
    location /api/live/ {
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_set_header Host $host;
        proxy_pass http://grafana:3000;
    }
}
```

**5. `docker-compose.yml`** (production — not the repo's dev compose file):

```yaml
services:
  grafana:
    image: grafana/grafana:12.3.0   # match or exceed plugin.json's grafanaDependency
    restart: unless-stopped
    ports:
      - '3000:3000'   # only exposed to nginx / the docker network, not the public internet
    environment:
      GF_SERVER_ROOT_URL: https://your-grafana-host/
      GF_PLUGIN_DATA_DIR: /var/lib/grafana/plugin-data/myorg-a2achat-app
    volumes:
      - grafana-storage:/var/lib/grafana
      - ./myorg-a2achat-app-dist:/var/lib/grafana/plugins/myorg-a2achat-app:ro
      # optional, for hands-off configuration — see provisioning/plugins/apps.yaml
      - ./provisioning:/etc/grafana/provisioning:ro
volumes:
  grafana-storage:
```

`./myorg-a2achat-app-dist` is the **signed** `dist/` output from steps 1–2,
copied to the host running this compose file.

**6. Start / reload**

```bash
docker compose up -d
# after replacing plugin files in an already-running deployment:
docker compose restart grafana
```

`plugin.json` changes always require a restart (same as in dev) — the
running Grafana process only reads it at plugin-load time.

**7. Verify**

```bash
docker compose logs grafana | grep myorg-a2achat-app
```

Expect `Plugin registered pluginId=myorg-a2achat-app` with **no** "Plugin is
unsigned" warning. In the UI: **Administration → Plugins → Grafana-chat**
should show a signed badge instead of the unsigned-plugin
warning banner.

**Troubleshooting**

- **Signature invalid / rootUrls mismatch** — re-sign with the exact
  `GF_SERVER_ROOT_URL` value, including the trailing slash and scheme
  (`http` vs `https`). As a last resort while diagnosing (not for real
  production use), `GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=myorg-a2achat-app`
  bypasses the check entirely.
- **`exec format error` / plugin fails to start** — the backend binary's
  architecture doesn't match the container host; rebuild with the matching
  `GOOS`/`GOARCH` (check the host with `docker exec <container> uname -m`).
- **Chat history disappears after a redeploy** — `GF_PLUGIN_DATA_DIR` isn't
  set, or isn't under a volume that persists across container recreation.

## Configuration

The same configuration steps apply whether you installed via the dev
`docker compose` setup above or the production install: In Grafana, go to
**Administration → Plugins → Grafana-chat → Configuration**:

| Setting | Description |
| --- | --- |
| **Agent endpoint URL** | The A2A JSON-RPC endpoint, e.g. `http://localhost:8000` or a LangGraph deployment's `/a2a/{assistant_id}` URL |
| **API Key** | Optional bearer token sent as `Authorization: Bearer ...` |
| **Stream responses** | On (default): stream via `message/stream`. Off: blocking `message/send` for agents without streaming support |

Use **Test connection** after saving — it fetches the agent card from
`{endpoint}/.well-known/agent-card.json` and shows the agent name, description,
and declared streaming capability.

Then open **More apps → Grafana-chat** and start chatting.

## How it works

- Each conversation in the sidebar maps 1:1 to an A2A `contextId`, so the agent
  remembers prior turns (deep agents keep their todos/tool state too).
- Only the new message is sent per turn; the agent's server-side state supplies
  the rest of the context.
- Every message is also stored in the plugin's SQLite DB (`data/chat.db` in dev),
  which is what the UI renders — history survives Grafana restarts and agent
  restarts. If the agent forgets a context (e.g. in-memory dev server restart),
  the plugin automatically starts a fresh context and keeps going.
- Conversations are titled automatically from the first message.

## Testing

```bash
npm run test:ci    # frontend unit tests (Jest)
npm run typecheck  # TypeScript
npm run lint       # ESLint
go test ./pkg/...  # backend unit tests
```

End-to-end tests (Playwright): `npm run e2e` (requires the docker Grafana).

## Project layout

```
pkg/plugin/       Go backend: resources.go (HTTP routes), a2a.go (A2A client),
                  store.go (SQLite), app.go (wiring)
src/pages/        ChatPage.tsx (main page)
src/components/   Chat/ (Sidebar, MessageList, Composer), AppConfig/
src/state/        useChat.ts (chat state + streaming lifecycle)
src/api/          client.ts (resource API + SSE parser)
```

## Notes

- `plugin.json` changes require a Grafana restart (`docker compose restart`).
- Frontend code is built with webpack (`.config/`), backend with mage — both are
  managed by Grafana's plugin tooling; do not edit `.config/` directly.
