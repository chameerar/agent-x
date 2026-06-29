# Agent X — a tiny AI agent in Go, from scratch

A learning project: a command-line AI agent built **from scratch in Go**, talking
to a **local LLM via [Ollama](https://ollama.com)** — no agent framework, just
`net/http`, `encoding/json`, and the agent loop written by hand.

The goal is to *understand* what an agent really is. Spoiler: it's a loop, some
HTTP calls, and a list of tools. Everything else is polish.

## What it does

- Multi-turn **chat** with conversation memory.
- **Tool calling**: the model can ask the program to run tools, and the program
  feeds results back until the model produces a final answer (the agent loop).
- Three built-in tools:
  | Tool | Description |
  |------|-------------|
  | `calculator` | Basic arithmetic (`add`, `subtract`, `multiply`, `divide`). |
  | `current_time` | The current local date and time. |
  | `read_file` | Reads a text file **sandboxed** to one directory. |
- Defensive by design: unknown-tool guard, malformed-output guard, bounded tool
  iterations, graceful per-turn error recovery, and a path-traversal-proof
  file sandbox.

## Prerequisites

- **Go 1.26+** (the module pins `toolchain go1.26.4`).
- **[Ollama](https://ollama.com)** running locally.
- A **tool-capable** model. `qwen2.5` is recommended; `llama3.2` works but is
  unreliable at deciding when to call tools.

```sh
ollama pull qwen2.5
```

## Quick start

```sh
go run . -model qwen2.5
```

```
Chat with qwen2.5 (tools: calculator, current_time, read_file). Type 'exit' or Ctrl-D to quit.

You: what is 4839 multiplied by 1271?
  [tool] calculator(map[a:4839 b:1271 operation:multiply]) => 6150369
AI:  4839 multiplied by 1271 is 6150369.
```

The `[tool] ...` lines show the agent loop in action: the model requested a
tool, the program ran it, and the result was fed back for the final answer.

## Usage

| Flag | Default | Description |
|------|---------|-------------|
| `-model` | `llama3.2` (env `OLLAMA_MODEL`) | Ollama model to use. |
| `-host` | `http://localhost:11434` (env `OLLAMA_HOST`) | Ollama server URL. |
| `-system` | built-in prompt | Override the system prompt. |
| `-sandbox` | `.` | Directory the `read_file` tool is restricted to. |
| `-serve` | `false` | Run the web chat UI instead of the CLI. |
| `-addr` | `localhost:8080` | Listen address for `-serve` mode. |
| `-otel` | `false` | Export traces to an OTLP collector (e.g. Jaeger). |
| `-otel-endpoint` | `localhost:4318` | OTLP/HTTP endpoint (host:port). |

## Web UI

The same agent, in the browser, with answers that **stream token-by-token**:

```sh
go run . -serve -model qwen2.5
# then open http://localhost:8080
```

The UI is a single `web/index.html` **embedded into the binary** (`//go:embed`),
served by Go's standard library — no JS framework, no build step. The browser
POSTs your message and reads back a stream of newline-delimited JSON events
(`token`, `tool_call`, `tool_result`, `done`), rendering text as it arrives and
showing tool activity inline. "New chat" clears the conversation.

The server binds to **`localhost` only** by default: the `read_file` tool runs
on behalf of anyone who can reach the port, so it is not exposed on the network.

It is the *same* `Agent` and tools as the CLI — only the front-end differs. See
`Client.ChatStream` and `Agent.AskStream` in `main.go`, and `server.go`.

## Tracing — watch the loop with OpenTelemetry

The agent loop maps naturally onto **distributed tracing**: a *turn* is a trace,
and every model round-trip and tool call is a nested *span*. With `-otel` on,
Agent X exports those spans over OTLP/HTTP so you can watch the loop as a
waterfall in [Jaeger](https://www.jaegertracing.io/).

Run Jaeger (its UI is on `16686`, its OTLP/HTTP receiver on `4318`):

```sh
docker run -d --name jaeger -e COLLECTOR_OTLP_ENABLED=true \
  -p 16686:16686 -p 4317:4317 -p 4318:4318 \
  jaegertracing/all-in-one:latest
```

Then run with tracing on (works in both CLI and `-serve` mode):

```sh
go run . -model qwen2.5 -otel
```

Open <http://localhost:16686>, pick the **agent-x** service, and a turn looks like:

```
agent.turn
 ├─ llm.chat          gen_ai.request.model, gen_ai.usage.input/output_tokens
 ├─ tool.calculator   tool.args={...}, tool.result=...
 └─ llm.chat          (the final, streamed answer)
```

How it is wired (all opt-in, see `tracing.go`):

- A `TracerProvider` exports spans to the OTLP endpoint; `initTracing` returns a
  shutdown func that **flushes** batched spans on exit (or the last trace is lost).
- Spans nest via `context.Context` — the same `ctx` already threaded through the
  loop carries the parent span, so children attach automatically.
- LLM spans use OpenTelemetry's **GenAI semantic conventions** (`gen_ai.*`),
  including real prompt/response **token counts** read from Ollama's response.
- Tool results are **truncated** before being recorded — telemetry backends
  retain what you send, so we never dump whole files or sensitive content.

When `-otel` is off there is no provider, so every span is a no-op and free.

## How it works — the agent loop

The heart of `Agent.Ask`:

```
loop (bounded):
  reply = model(history + tool definitions)
  if reply has no tool calls:  return reply text   // done
  for each tool call:
      result = run the tool
      append result to history                      // feed it back
  // loop again so the model sees the result
```

That loop *is* the agent. Frameworks dress it up; this is the core.

## Project structure

```
main.go         Client (HTTP to Ollama), Agent (the loop), CLI, config
server.go       Web mode: streaming HTTP handlers + embedded UI
web/index.html  Single-file browser chat (vanilla HTML/CSS/JS)
tracing.go      OpenTelemetry setup (opt-in -otel): exporter, provider, flush
tools.go        Tool/Toolbox types and the three tool implementations
tools_test.go   Unit tests (calculator + the read_file sandbox guard)
```

## Testing

```sh
go test ./...
```

The tests assert calculator behavior (including string-coerced operands and
divide-by-zero) and prove the `read_file` sandbox blocks path traversal and
absolute-path escapes.

## Notes on model behavior

Three independent levers control how an agent behaves:

| Lever | Controls | Example |
|-------|----------|---------|
| **Model choice** | Whether/which tool to call | `qwen2.5` decides well; `llama3.2` over-calls |
| **System prompt** | The policy for when tools apply | "never guess a value a tool can give you" |
| **Defensive code** | What happens when the model misbehaves | sandbox guard, error feedback, iteration cap |

Security rules (like the file sandbox) live in **code**, never in the prompt —
a model can ignore a prompt, but it cannot bypass a path check in Go.

Multi-step *autonomous* tool chaining (read → fetch → calculate without a nudge)
improves sharply with larger models; small 3B models often guess instead.

---

*A personal learning project — built phase by phase.*
