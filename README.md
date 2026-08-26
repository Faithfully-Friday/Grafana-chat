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

## Configuration

In Grafana, go to **Administration → Plugins → A2a-Chat → Configuration**:

| Setting | Description |
| --- | --- |
| **Agent endpoint URL** | The A2A JSON-RPC endpoint, e.g. `http://localhost:8000` or a LangGraph deployment's `/a2a/{assistant_id}` URL |
| **API Key** | Optional bearer token sent as `Authorization: Bearer ...` |
| **Stream responses** | On (default): stream via `message/stream`. Off: blocking `message/send` for agents without streaming support |

Use **Test connection** after saving — it fetches the agent card from
`{endpoint}/.well-known/agent-card.json` and shows the agent name, description,
and declared streaming capability.

Then open **More apps → A2a-Chat** and start chatting.

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
