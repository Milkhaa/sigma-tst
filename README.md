# sigma-tst — Hybrid Test Execution Prototype

A runnable demonstration of hybrid test execution: natural language intent → structured test → deterministic Playwright run → AI-powered fallback recovery → human-reviewed promotion back to deterministic.


---

## What it demonstrates

- **Capture** — type a plain-English test intent; LLM generates a structured, grounded test spec
- **Run** — Playwright executes the spec deterministically against any real URL
- **Fallback** — one step is force-injected with a stale selector; the runner detects the failure, calls the LLM, and recovers the intent
- **Promote** — the recovered selector is surfaced as a promotion candidate; approve it and the spec is updated so the next run is fully deterministic again

---

## Prerequisites

| Tool | Version |
|------|---------|
| Go | 1.22 or later |
| Node.js | 18 or later |
| npm | bundled with Node |
| An LLM API key | OpenAI **or** Anthropic — required|

---

## Quick start 

### 1. Install dependencies

```bash
# From the repo root

cd runner && npm install && npx playwright install chromium && cd ..

cd frontend && npm install && cd ..

cd backend && go mod download && cd ..
```

### 2. Configure the backend

```bash
cp backend/.env.example backend/.env
```

Open `backend/.env` and fill in your API key:

```dotenv
# Pick one provider
OPENAI_API_KEY=<sk-... >        
OPENAI_MODEL=gpt-4o-mini

# OR
ANTHROPIC_API_KEY=<sk-ant-...>
LLM_PROVIDER=anthropic

PORT=8080
```


### 3. Configure the frontend

```bash
cp frontend/.env.local.example frontend/.env.local
```

The default (`NEXT_PUBLIC_API_BASE=http://localhost:8080`) is correct for local use.

### 4. Start the backend

```bash
cd backend
go run ./cmd/main.go
```

You should see: `API listening port=8080`

### 5. Start the frontend

```bash
cd frontend
npm run dev
```

Open **http://localhost:3000**

---


## API endpoints

| Method | Path | What it does |
|--------|------|--------------|
| `POST` | `/v1/tests/generate` | LLM generates a grounded `TestSpec` from intent |
| `POST` | `/v1/tests/run` | Executes a `TestSpec`; returns `RunResult` with per-step traces |
| `POST` | `/v1/promotions/:id/approve` | Applies recovered selector to spec; saves `promoted-spec.json` |
| `POST` | `/v1/promotions/:id/reject` | Marks candidate rejected |
| `GET` | `/health` | Liveness check |


## Improvements required:
- Add integration tests
- Add more unit tests for coverage 
- Auth setup : local keycloak setup + app expecting OAuth Token
- DB: local db setup + app persistance layer
- Improvise this prototype to be more closer to Architecture diagram 
