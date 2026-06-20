# Building an AI Agent from Scratch (Go + Ollama)

A hands-on learning project: build a chatbot first, then grow it into a real agent —
writing every piece by hand so there's no framework "magic."

## Stack & choices
- **Language:** Go (1.25.5 installed)
- **LLM:** Local model via [Ollama](https://ollama.com) (free, private, no API keys)
- **Approach:** From scratch — raw HTTP + JSON, no agent SDK
- **Path:** Simple chatbot → agent

## The one idea that demystifies everything
> An LLM is **stateless**. It's just a function: `text in → text out`. It has no memory,
> can't "do" anything, and forgets you the instant it replies. Everything that makes
> something feel like a chatbot or agent is **code we write around that function** —
> keeping history, looping, and running tools.

## Roadmap

```
Phase 0  Setup            Install Ollama, pull a model, talk to it with curl
Phase 1  One-shot         Go program: send 1 prompt -> print 1 reply   (HTTP + JSON)
Phase 2  Chatbot          Loop + message history -> it "remembers"      (conversation state)
Phase 3  Streaming        Print tokens as they arrive                  (JSON lines)
Phase 4  Personality      System prompt, config, clean code structure
--------  ^ that's a chatbot ----------------------  v now it becomes an agent --------
Phase 5  The agent loop   Give the model TOOLS it can ask us to run    <- the real lesson
Phase 6  Real tools       Calculator, read file, etc. + multi-step reasoning
Phase 7  (stretch)        Memory, simple RAG over your own notes
```

### What each phase teaches
- **Phase 0 — Setup:** Install Ollama, pull a small capable model (`llama3.2` or `qwen2.5`).
  Test with `curl` to see the raw HTTP before any Go.
- **Phase 1 — One-shot:** ~40 lines of Go hitting `/api/generate`. Learn request structs,
  JSON marshal/unmarshal, reading the response. *The LLM is just an HTTP endpoint.*
- **Phase 2 — Chatbot:** Loop reading stdin; keep a `[]Message` slice and resend the whole
  history each turn via `/api/chat`. *"Memory" is just replaying the transcript.*
- **Phase 3 — Streaming:** Read newline-delimited JSON, print as it generates.
- **Phase 4 — Personality & structure:** System prompt, config, split into small functions.
- **Phase 5 — The agent loop (the payoff):**
  ```
  1. Send messages + tool definitions to the model
  2. Model replies EITHER with text (done) OR "call tool X with these args"
  3. If tool call -> our Go code runs the tool, appends the result to messages
  4. Loop back to step 1 until the model returns plain text
  ```
  *This loop IS an agent. Frameworks just dress it up.*
- **Phase 6 — Real tools:** 2-3 actual tools (calculator, file reader, HTTP fetch);
  watch it chain steps to answer one question.
- **Phase 7 — Stretch:** Persist history to disk; tiny RAG over your own notes.

## Working agreement
- One phase at a time. Explain -> build -> run -> ask questions -> next.
- You run the `ollama` and `go run` commands yourself (via `! <command>`) so the learning sticks.

## Progress
- [x] Phase 0 — Setup
- [x] Phase 1 — One-shot
- [x] Phase 2 — Chatbot
- [x] Phase 3 — Streaming
- [x] Phase 4 — Personality
- [x] Phase 5 — Agent loop
- [x] Phase 6 — Real tools
- [ ] Phase 7 — Stretch (optional)
