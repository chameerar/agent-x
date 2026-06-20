package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunCalculator(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		want    string
		wantErr bool
	}{
		{"add", map[string]any{"operation": "add", "a": 2.0, "b": 3.0}, "5", false},
		{"multiply", map[string]any{"operation": "multiply", "a": 7.0, "b": 9.0}, "63", false},
		{"string operands", map[string]any{"operation": "add", "a": "10", "b": "2"}, "12", false},
		{"divide by zero", map[string]any{"operation": "divide", "a": 1.0, "b": 0.0}, "", true},
		{"bad operation", map[string]any{"operation": "pow", "a": 2.0, "b": 3.0}, "", true},
		{"non-numeric", map[string]any{"operation": "add", "a": "x", "b": 3.0}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := runCalculator(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadFileInDirSandbox(t *testing.T) {
	// A throwaway sandbox with one file, plus a secret file OUTSIDE it.
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "hello.txt"), []byte("hi there"), 0o600); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(filepath.Dir(base), "secret.txt")
	if err := os.WriteFile(secret, []byte("top secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(secret) })

	t.Run("reads file inside sandbox", func(t *testing.T) {
		got, err := readFileInDir(base, "hello.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "hi there" {
			t.Errorf("got %q, want %q", got, "hi there")
		}
	})

	t.Run("blocks path traversal", func(t *testing.T) {
		if _, err := readFileInDir(base, "../secret.txt"); err == nil {
			t.Fatal("expected traversal to be denied, got nil error")
		}
	})

	t.Run("blocks absolute escape", func(t *testing.T) {
		if _, err := readFileInDir(base, "/etc/hosts"); err == nil {
			t.Fatal("expected absolute path outside sandbox to be denied")
		}
	})
}
