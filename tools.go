package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ToolSpec follows the JSON-schema "function" shape Ollama expects for tools.
type ToolSpec struct {
	Type     string       `json:"type"` // always "function"
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON schema for the args
}

type Tool struct {
	Spec ToolSpec
	Run  func(args map[string]any) (string, error)
}

type Toolbox struct {
	tools map[string]Tool
}

func NewToolbox(tools ...Tool) *Toolbox {
	m := make(map[string]Tool, len(tools))
	for _, t := range tools {
		m[t.Spec.Function.Name] = t
	}
	return &Toolbox{tools: m}
}

func (tb *Toolbox) Specs() []ToolSpec {
	specs := make([]ToolSpec, 0, len(tb.tools))
	for _, t := range tb.tools {
		specs = append(specs, t.Spec)
	}
	return specs
}

func (tb *Toolbox) Names() []string {
	names := make([]string, 0, len(tb.tools))
	for name := range tb.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (tb *Toolbox) Run(name string, args map[string]any) (string, error) {
	t, ok := tb.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool %q", name)
	}
	return t.Run(args)
}

func calculatorTool() Tool {
	return Tool{
		Spec: ToolSpec{
			Type: "function",
			Function: ToolFunction{
				Name:        "calculator",
				Description: "Perform basic arithmetic on two numbers.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"operation": map[string]any{
							"type":        "string",
							"enum":        []string{"add", "subtract", "multiply", "divide"},
							"description": "The operation to perform.",
						},
						"a": map[string]any{"type": "number", "description": "First operand."},
						"b": map[string]any{"type": "number", "description": "Second operand."},
					},
					"required": []string{"operation", "a", "b"},
				},
			},
		},
		Run: runCalculator,
	}
}

func runCalculator(args map[string]any) (string, error) {
	op, _ := args["operation"].(string)
	a, ok1 := toFloat(args["a"])
	b, ok2 := toFloat(args["b"])
	if !ok1 || !ok2 {
		return "", fmt.Errorf("a and b must be numbers")
	}

	var result float64
	switch op {
	case "add":
		result = a + b
	case "subtract":
		result = a - b
	case "multiply":
		result = a * b
	case "divide":
		if b == 0 {
			return "", fmt.Errorf("division by zero")
		}
		result = a / b
	default:
		return "", fmt.Errorf("unknown operation %q", op)
	}
	return fmt.Sprintf("%v", result), nil
}

func currentTimeTool() Tool {
	return Tool{
		Spec: ToolSpec{
			Type: "function",
			Function: ToolFunction{
				Name:        "current_time",
				Description: "Get the current local date and time.",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		},
		Run: func(map[string]any) (string, error) {
			return time.Now().Format("2006-01-02 15:04:05 MST"), nil
		},
	}
}

// maxFileBytes bounds how much of a file we'll read, so a huge file can't
// exhaust memory or blow past the model's context window.
const maxFileBytes = 8 * 1024

// readFileTool reads a text file, but ONLY within baseDir. The model is
// untrusted input: it may ask for "../../etc/passwd", so every path is resolved
// and checked to stay inside the sandbox before we touch the disk.
func readFileTool(baseDir string) Tool {
	return Tool{
		Spec: ToolSpec{
			Type: "function",
			Function: ToolFunction{
				Name:        "read_file",
				Description: "Read a UTF-8 text file from the sandbox directory. Provide a relative path.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "Path to the file, relative to the sandbox directory.",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		Run: func(args map[string]any) (string, error) {
			rel, _ := args["path"].(string)
			return readFileInDir(baseDir, rel)
		},
	}
}

func readFileInDir(baseDir, rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", fmt.Errorf("path is required")
	}

	base, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolving sandbox: %w", err)
	}

	// Join then verify the result is still inside base. filepath.Join cleans
	// the path, so a "../" in rel resolves here and the prefix check catches
	// any escape attempt.
	full := filepath.Join(base, rel)
	if full != base && !strings.HasPrefix(full, base+string(os.PathSeparator)) {
		return "", fmt.Errorf("access denied: %q is outside the sandbox", rel)
	}

	f, err := os.Open(full)
	if err != nil {
		return "", fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxFileBytes))
	if err != nil {
		return "", fmt.Errorf("reading file: %w", err)
	}
	return string(data), nil
}

// toFloat coerces a JSON value to float64. Models often send numbers as
// strings ("10"), so we accept those too.
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case int:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	default:
		return 0, false
	}
}
