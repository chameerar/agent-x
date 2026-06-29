# Agent X — a tiny AI agent in Go, from scratch

A command-line **and** web AI agent built from scratch in Go against a local LLM
via [Ollama](https://ollama.com) — no agent framework, just `net/http`,
`encoding/json`, and the agent loop written by hand. Instrumented with
**OpenTelemetry**, so every turn is a trace you can watch.

The point is to show what an agent *actually* is: a bounded loop, some HTTP
calls, and a list of tools. Everything else is polish.

## The agent loop

The whole thing is `Agent.Ask`:

```
loop (bounded):
  reply = model(history + tool definitions)
  if reply has no tool calls:  return reply text     // done
  for each tool call:
      result = run the tool
      append result to history                       // feed it back
  // loop again so the model sees the result
```

That loop *is* the agent. A `maxToolIterations` cap keeps a confused model from
spinning forever.

## Tools

| Tool | Description |
|------|-------------|
| `calculator` | Arithmetic (`add`, `subtract`, `multiply`, `divide`). |
| `current_time` | Current local date and time. |
| `read_file` | Reads a text file **sandboxed** to one directory. |

Defensive by design: unknown-tool guard, malformed-output guard, bounded
iterations, and per-turn errors handed back to the model as text it can recover
from. Security rules live in **code, never the prompt** — a model can ignore a
prompt, but it can't bypass a path-traversal check in Go.

## Run it

Needs [Ollama](https://ollama.com) and a tool-capable model (`ollama pull qwen2.5`).

```sh
go run . -model qwen2.5            # CLI chat
go run . -serve -model qwen2.5     # web UI at http://localhost:8080
```

`go run . -h` lists all flags (`-host`, `-system`, `-sandbox`, `-addr`, …).

### With Docker (no Go toolchain)

```sh
cp .env.example .env
docker compose up --build                      # talks to an Ollama you already run
docker compose --profile bundled up --build    # or run Ollama in Docker too
```

`.env` sets the model, port, and where to find Ollama. **Bundled** mode pulls the
model on first start and runs Ollama CPU-only (slower — comment out `OLLAMA_HOST`
first). The web UI binds `127.0.0.1` only, since `read_file` acts for anyone who
can reach the port. Stop with `docker compose down` (`-v` also drops the model
volume).

## Web UI

The same `Agent` and tools, streaming **token-by-token** in the browser.
`web/index.html` is a single file **embedded into the binary** (`//go:embed`) and
served by the standard library — no JS framework, no build step. The browser
POSTs a message and reads back newline-delimited JSON events (`token`,
`tool_call`, `tool_result`, `done`), rendering text as it arrives. See
`Agent.AskStream` and `server.go`.

## Tracing with OpenTelemetry

The agent loop maps cleanly onto distributed tracing: a **turn is a trace**, and
each model round-trip and tool call is a nested **span**.

```sh
OTEL=true docker compose --profile tracing up --build   # agent + Jaeger
# or locally: run Jaeger, then `go run . -model qwen2.5 -otel`
```

Open <http://localhost:16686>, pick the **agent-x** service, and a turn reads:

```
agent.turn
 ├─ llm.chat          gen_ai.request.model, gen_ai.usage.input/output_tokens
 ├─ tool.calculator   tool.args={...}, tool.result=...
 └─ llm.chat          (the final, streamed answer)
```

How it's wired (`tracing.go`):

- A `TracerProvider` exports over OTLP/HTTP; `initTracing` returns a shutdown func
  that **flushes** batched spans on exit (or the last trace is lost).
- Spans nest via `context.Context` — the same `ctx` threaded through the loop
  carries the parent span, so children attach automatically.
- LLM spans follow the **GenAI semantic conventions** (`gen_ai.*`), with real
  prompt/response **token counts** read from Ollama's response.
- Tool results are **truncated** before recording — backends keep what you send,
  so no whole files or secrets land in telemetry.

Off by default: with no provider installed, every span is a no-op and free.

## What controls an agent's behavior

Three independent levers — worth separating, because they fail differently:

| Lever | Controls | Example |
|-------|----------|---------|
| **Model choice** | Whether/which tool to call | `qwen2.5` decides well; small 3B models guess |
| **System prompt** | The policy for when tools apply | "never guess a value a tool can give you" |
| **Defensive code** | What happens when the model misbehaves | sandbox guard, error feedback, iteration cap |

Autonomous multi-step chaining (read → compute → answer) improves sharply with
larger models; 3B models often emit malformed tool args or skip the tool — both
visible in the traces above.

## Layout

```
main.go         Client (HTTP to Ollama), Agent (the loop), CLI, config
server.go       Web mode: streaming HTTP handlers + embedded UI
web/index.html  Single-file browser chat (vanilla HTML/CSS/JS)
tracing.go      OpenTelemetry setup (opt-in): exporter, provider, flush
tools.go        Tool/Toolbox types and the three tools
tools_test.go   Calculator + read_file sandbox tests
Dockerfile      Multi-stage build → tiny static (distroless) image
compose.yaml    Agent; Ollama and Jaeger behind --profile bundled/tracing
```

```sh
go test ./...   # calculator behavior + read_file sandbox escapes
```

---

*A personal learning project — built phase by phase.*
