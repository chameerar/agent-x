# Gopher — a tiny AI agent in Go, from scratch

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
main.go        Client (HTTP to Ollama), Agent (the loop), CLI, config
tools.go       Tool/Toolbox types and the three tool implementations
tools_test.go  Unit tests (calculator + the read_file sandbox guard)
PLAN.md        The phased learning roadmap this was built against
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

*A personal learning project — built phase by phase. See `PLAN.md` for the roadmap.*
