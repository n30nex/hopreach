package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMeshMapperKeyFileAndEnvironmentDoNotEnterConfigJS(t *testing.T) {
	t.Setenv("MESHMAPPER_API_KEY", "")
	p := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(p, []byte(" fixture-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if key, err := meshMapperKey(p); err != nil || key != "fixture-secret" {
		t.Fatalf("key file read failed: %v", err)
	}
	t.Setenv("MESHMAPPER_API_KEY", "override-secret")
	if key, err := meshMapperKey(p); err != nil || key != "override-secret" {
		t.Fatal("environment override failed")
	}
	source, err := os.ReadFile("prepare.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), "APIKeyFile") || strings.Contains(string(source), "MESHMAPPER_API_KEY") {
		t.Fatal("private key plumbing referenced by public config renderer")
	}
}
