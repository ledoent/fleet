// Package pgcompat_test is a regression test for the PG-compat CI gate.
// It exercises the two validators that run in validate-pg-compat.yml with
// inputs that should fail, and asserts they exit non-zero. If a validator
// is silently disabled or its exit code regresses, this test catches it
// before the gate becomes a no-op.
package pgcompat_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot returns the repo root by walking up from this test file.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found above %s", wd)
		}
		dir = parent
	}
}

func TestSchemaDriftValidator_FailsOnEmptyAllowlist(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	empty := filepath.Join(tmp, "allowlist.txt")
	if err := os.WriteFile(empty, []byte("# intentionally empty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "run", "./tools/pgcompat/check_schema_drift",
		"-allowlist", empty)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit when allowlist is empty (drift exists), got success.\nOutput: %s", out)
	}
	if !strings.Contains(string(out), "NOT in") {
		t.Fatalf("expected drift diagnostic in output, got: %s", out)
	}
}

func TestSchemaDriftValidator_PassesWithRealAllowlist(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("go", "run", "./tools/pgcompat/check_schema_drift")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("validator failed against checked-in inputs: %v\nOutput: %s", err, out)
	}
	if !strings.HasPrefix(string(out), "OK:") {
		t.Fatalf("expected OK prefix, got: %s", out)
	}
}

func TestPrimaryKeysValidator_PassesWithRealInputs(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("go", "run", "./tools/pgcompat/check_primary_keys")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("validator failed against checked-in inputs: %v\nOutput: %s", err, out)
	}
}
