package main

import (
	"encoding/json"
	"fmt"
)

// ToolSpec is the definition we hand to the model so it knows a tool exists.
// It follows the JSON-schema "function" shape that Ollama (and most LLM APIs)
// expect.
type ToolSpec struct {
	Type     string       `json:"type"` // always "function"
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON schema for the args
}

// Tool couples a definition (what the model sees) with an implementation
// (the Go code we run when the model calls it).
type Tool struct {
	Spec ToolSpec
	Run  func(args map[string]any) (string, error)
}

// Toolbox is a name-indexed registry of tools.
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

// Specs returns every tool definition, to send to the model.
func (tb *Toolbox) Specs() []ToolSpec {
	specs := make([]ToolSpec, 0, len(tb.tools))
	for _, t := range tb.tools {
		specs = append(specs, t.Spec)
	}
	return specs
}

// Run executes the named tool with the given arguments.
func (tb *Toolbox) Run(name string, args map[string]any) (string, error) {
	t, ok := tb.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool %q", name)
	}
	return t.Run(args)
}

// calculatorTool performs basic arithmetic — something small models get wrong
// surprisingly often, which makes it a perfect first tool.
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

// toFloat coerces a JSON-decoded value into a float64. JSON numbers decode to
// float64, but we accept a couple of other shapes defensively.
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}
