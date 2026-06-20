package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const defaultSystemPrompt = "You are Gopher, a concise and friendly assistant. " +
	"You have three tools: calculator (arithmetic), current_time (the date/time), " +
	"and read_file (read a file from the sandbox).\n" +
	"Rules:\n" +
	"- NEVER guess or make up a value that a tool can give you. Use calculator " +
	"for ALL arithmetic, current_time for the date/time, and read_file for file " +
	"contents — even if you think you know the answer.\n" +
	"- If a task needs several steps (e.g. read a file, then get the time, then " +
	"calculate), call the tools one at a time, using each result, before you answer.\n" +
	"- Do not call a tool just to demonstrate it, and do not invent file paths.\n" +
	"- If asked what tools you have, describe them in words instead of calling them.\n" +
	"Keep answers short."

// maxToolIterations caps how many tool round-trips one user turn may trigger,
// so a confused model can't loop forever.
const maxToolIterations = 5

type Message struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	ToolName  string     `json:"tool_name,omitempty"` // set on role:"tool" replies
}

// ToolCall is the model asking us to run a tool.
type ToolCall struct {
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type chatRequest struct {
	Model    string     `json:"model"`
	Messages []Message  `json:"messages"`
	Stream   bool       `json:"stream"`
	Tools    []ToolSpec `json:"tools,omitempty"`
}

type chatResponse struct {
	Message Message `json:"message"`
	Done    bool    `json:"done"`
}

type Config struct {
	Host    string
	Model   string
	System  string
	Sandbox string
}

type Client struct {
	http  *http.Client
	host  string
	model string
}

func NewClient(host, model string) *Client {
	return &Client{
		http:  &http.Client{Timeout: 60 * time.Second},
		host:  host,
		model: model,
	}
}

// Agent owns a conversation, a client, and a set of tools.
type Agent struct {
	client  *Client
	toolbox *Toolbox
	history []Message
}

func NewAgent(client *Client, toolbox *Toolbox, system string) *Agent {
	return &Agent{
		client:  client,
		toolbox: toolbox,
		history: []Message{{Role: "system", Content: system}},
	}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseConfig() Config {
	var cfg Config
	flag.StringVar(&cfg.Host, "host", envOr("OLLAMA_HOST", "http://localhost:11434"), "Ollama server URL")
	flag.StringVar(&cfg.Model, "model", envOr("OLLAMA_MODEL", "llama3.2"), "model name")
	flag.StringVar(&cfg.System, "system", defaultSystemPrompt, "system prompt")
	flag.StringVar(&cfg.Sandbox, "sandbox", ".", "directory the read_file tool is restricted to")
	flag.Parse()
	return cfg
}

// Chat sends the conversation plus available tools and returns the model's
// reply, which may be plain text or a request to call tools.
func (c *Client) Chat(ctx context.Context, messages []Message, tools []ToolSpec) (Message, error) {
	payload, err := json.Marshal(chatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   false,
		Tools:    tools,
	})
	if err != nil {
		return Message{}, fmt.Errorf("encoding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+"/api/chat", bytes.NewReader(payload))
	if err != nil {
		return Message{}, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return Message{}, fmt.Errorf("calling ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return Message{}, fmt.Errorf("ollama returned %s: %s", resp.Status, body)
	}

	var result chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return Message{}, fmt.Errorf("decoding response: %w", err)
	}
	return result.Message, nil
}

// Ask runs one user turn through the agent loop: keep calling the model and
// running any tools it requests until it returns a plain text answer.
func (a *Agent) Ask(ctx context.Context, input string) (string, error) {
	a.history = append(a.history, Message{Role: "user", Content: input})

	for range maxToolIterations {
		reply, err := a.client.Chat(ctx, a.history, a.toolbox.Specs())
		if err != nil {
			return "", err
		}
		a.history = append(a.history, reply)

		if len(reply.ToolCalls) == 0 {
			if looksLikeToolCallLeak(reply.Content) {
				a.history = append(a.history, Message{
					Role:    "user",
					Content: "Please answer in plain text, or make a proper tool call.",
				})
				continue
			}
			return reply.Content, nil
		}

		// Run each requested tool and feed the results back in.
		for _, call := range reply.ToolCalls {
			result := a.runToolCall(call)
			fmt.Printf("  [tool] %s(%v) => %s\n", call.Function.Name, call.Function.Arguments, result)
			a.history = append(a.history, Message{
				Role:     "tool",
				Content:  result,
				ToolName: call.Function.Name,
			})
		}
	}

	// Exhausted the tool budget. Don't crash the session — return gracefully.
	return "I got stuck trying to use my tools. Could you rephrase that?", nil
}

// runToolCall executes one tool call. Unknown tools and tool errors are turned
// into messages the model can read and recover from — never a crash. The error
// text lists the real tools so the model can correct itself.
func (a *Agent) runToolCall(call ToolCall) string {
	result, err := a.toolbox.Run(call.Function.Name, call.Function.Arguments)
	if err != nil {
		return fmt.Sprintf("error: %v (available tools: %s)", err, strings.Join(a.toolbox.Names(), ", "))
	}
	return result
}

func looksLikeToolCallLeak(content string) bool {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "{") {
		return false
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &probe); err != nil {
		return false
	}
	_, hasName := probe["name"]
	_, hasParams := probe["parameters"]
	return hasName && hasParams
}

func run() error {
	cfg := parseConfig()
	client := NewClient(cfg.Host, cfg.Model)
	toolbox := NewToolbox(
		calculatorTool(),
		currentTimeTool(),
		readFileTool(cfg.Sandbox),
	)
	agent := NewAgent(client, toolbox, cfg.System)

	fmt.Printf("Chat with %s (tools: %s). Type 'exit' or Ctrl-D to quit.\n",
		cfg.Model, strings.Join(toolbox.Names(), ", "))

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\nYou: ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			break
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		answer, err := agent.Ask(ctx, input)
		cancel()
		if err != nil {
			// One failed turn (timeout, server hiccup) shouldn't end the chat.
			fmt.Fprintf(os.Stderr, "  (turn failed: %v)\n", err)
			continue
		}

		fmt.Printf("AI:  %s\n", strings.TrimSpace(answer))
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading input: %w", err)
	}
	fmt.Println("\nBye.")
	return nil
}
