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
	// Symmetric with the schema-drift / column-drift pass-tests: the tool
	// must emit an `OK:` line so a silent regression that erases all output
	// still fails the test.
	if !strings.HasPrefix(string(out), "OK:") {
		t.Fatalf("expected OK prefix, got: %s", out)
	}
}

func TestColumnDriftValidator_FailsOnSyntheticDrift(t *testing.T) {
	// Schema-drift's analogue relies on real PG-only tables to detect drift,
	// but if the column-level baseline is clean (the intended state after a
	// regen), there's no real drift to find. Use synthetic schemas to verify
	// the validator still detects column-level drift end-to-end.
	root := repoRoot(t)
	tmp := t.TempDir()

	mysqlFixture := filepath.Join(tmp, "schema.sql")
	if err := os.WriteFile(mysqlFixture, []byte(
		"CREATE TABLE `widgets` (\n"+
			"  `id` int NOT NULL,\n"+
			"  `name` varchar(255) NOT NULL,\n"+
			"  `mysql_only_col` int NOT NULL\n"+
			") ENGINE=InnoDB;\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	pgFixture := filepath.Join(tmp, "baseline.sql")
	if err := os.WriteFile(pgFixture, []byte(
		"CREATE TABLE public.widgets (\n"+
			"    id integer NOT NULL,\n"+
			"    name varchar(255) NOT NULL,\n"+
			"    pg_only_col integer NOT NULL\n"+
			");\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	empty := filepath.Join(tmp, "allowlist.txt")
	if err := os.WriteFile(empty, []byte("# intentionally empty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "run", "./tools/pgcompat/check_column_drift",
		"-mysql", mysqlFixture,
		"-pg", pgFixture,
		"-allowlist", empty)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit when synthetic drift exists, got success.\nOutput: %s", out)
	}
	if !strings.Contains(string(out), "Column drift") {
		t.Fatalf("expected drift diagnostic in output, got: %s", out)
	}
	if !strings.Contains(string(out), "mysql_only_col") {
		t.Fatalf("expected mysql_only_col in diagnostic, got: %s", out)
	}
	if !strings.Contains(string(out), "pg_only_col") {
		t.Fatalf("expected pg_only_col in diagnostic, got: %s", out)
	}
}

func TestColumnDriftValidator_PassesWithRealAllowlist(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("go", "run", "./tools/pgcompat/check_column_drift")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("validator failed against checked-in inputs: %v\nOutput: %s", err, out)
	}
	if !strings.HasPrefix(string(out), "OK:") {
		t.Fatalf("expected OK prefix, got: %s", out)
	}
}

func TestConstraintDriftValidator_FailsOnEmptyAllowlist(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	empty := filepath.Join(tmp, "allowlist.txt")
	if err := os.WriteFile(empty, []byte("# intentionally empty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "run", "./tools/pgcompat/check_constraint_drift",
		"-allowlist", empty)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit with empty allowlist (FK drift is deferred, so drift exists), got success.\nOutput: %s", out)
	}
	if !strings.Contains(string(out), "missing from PG baseline") {
		t.Fatalf("expected drift diagnostic in output, got: %s", out)
	}
}

func TestConstraintDriftValidator_PassesWithRealAllowlist(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("go", "run", "./tools/pgcompat/check_constraint_drift")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("validator failed against checked-in inputs: %v\nOutput: %s", err, out)
	}
	if !strings.HasPrefix(string(out), "OK:") {
		t.Fatalf("expected OK prefix, got: %s", out)
	}
}

func TestBoolColSplitValidator_FailsOnEmptyAllowlist(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	empty := filepath.Join(tmp, "allowlist.txt")
	if err := os.WriteFile(empty, []byte("# empty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", "./tools/pgcompat/check_bool_col_split", "-allowlist", empty)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected failure (awaiting_configuration split exists), got success: %s", out)
	}
	if !strings.Contains(string(out), "awaiting_configuration") {
		t.Fatalf("expected the split column in output, got: %s", out)
	}
}

func TestBoolColSplitValidator_PassesWithRealAllowlist(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("go", "run", "./tools/pgcompat/check_bool_col_split")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("validator failed against checked-in inputs: %v\n%s", err, out)
	}
}

// TestPrimaryKeysValidator_CatchesBadColumns proves the column-verification
// half of check_primary_keys: an entry whose columns match no PK/UNIQUE in
// the baseline (the class that produced two wrong entries fixed in Phase 2 of
// the review remediation) must fail the run.
func TestPrimaryKeysValidator_CatchesBadColumns(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	// Minimal fixture root: a driver file with one wrong-column entry, a
	// baseline where the real PK differs, and empty source trees.
	driverRel := "driver.go"
	if err := os.WriteFile(filepath.Join(tmp, driverRel), []byte(
		"package p\n\nvar knownPrimaryKeys = map[string]string{\n\t\"widgets\": \"id\",\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "server/datastore/mysql"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"server", "ee", "cmd"} {
		if err := os.MkdirAll(filepath.Join(tmp, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	baseline := "CREATE TABLE public.widgets (\n    a integer NOT NULL,\n    b integer NOT NULL\n);\n" +
		"ALTER TABLE ONLY public.widgets\n    ADD CONSTRAINT widgets_pkey PRIMARY KEY (a, b);\n"
	if err := os.WriteFile(filepath.Join(tmp, "server/datastore/mysql/pg_baseline_schema.sql"), []byte(baseline), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", "./tools/pgcompat/check_primary_keys", "-root", tmp, "-driver", driverRel)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected failure for wrong-column entry, got success: %s", out)
	}
	if !strings.Contains(string(out), "widgets") || !strings.Contains(string(out), "42P10") {
		t.Fatalf("expected widgets 42P10 diagnostic, got: %s", out)
	}
}
