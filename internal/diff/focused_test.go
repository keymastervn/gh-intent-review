package diff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultOutputPath(t *testing.T) {
	path := DefaultOutputPath("acme", "api", 42)
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".gh-intent-review", "acme", "api", "42.intentional.diff")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestProjectOutputPath(t *testing.T) {
	path := ProjectOutputPath(".reviews", "acme", "api", 42)
	expected := filepath.Join(".reviews", "acme", "api", "42.intentional.diff")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestParseIntentionalDiff_SingleIntent(t *testing.T) {
	input := `diff --git a/handler.go b/handler.go
--- a/handler.go
+++ b/handler.go
@@ -1,3 +1,4 @@
 package handler
+  db.Exec("SELECT *" + id)
 }

¿!! b/handler.go
@@ +2,1 @@
+  db.Exec("SELECT *" + id)
-{¿! SQL injection — use parameterized queries ¿!}
 }
`
	fd, err := ParseIntentionalDiff(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have raw diff
	if !strings.Contains(fd.RawDiff, "diff --git") {
		t.Error("expected RawDiff to contain the original diff")
	}

	// Should have 1 file with 1 intent
	if len(fd.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(fd.Files))
	}
	if fd.Files[0].Path != "handler.go" {
		t.Errorf("expected path 'handler.go', got %q", fd.Files[0].Path)
	}
	if len(fd.Files[0].Intents) != 1 {
		t.Fatalf("expected 1 intent, got %d", len(fd.Files[0].Intents))
	}

	intent := fd.Files[0].Intents[0]
	if intent.Symbol != "!" {
		t.Errorf("expected symbol '!', got %q", intent.Symbol)
	}
	if intent.Severity != "critical" {
		t.Errorf("expected severity 'critical', got %q", intent.Severity)
	}
	if !strings.Contains(intent.Explanation, "SQL injection") {
		t.Errorf("expected explanation to contain 'SQL injection', got %q", intent.Explanation)
	}
	if len(intent.AffectedLines) != 1 {
		t.Fatalf("expected 1 affected line, got %d", len(intent.AffectedLines))
	}
	if !strings.Contains(intent.AffectedLines[0], "db.Exec") {
		t.Errorf("expected affected line to contain 'db.Exec', got %q", intent.AffectedLines[0])
	}
	if intent.HunkHeader != "@@ +2,1 @@" {
		t.Errorf("expected hunk header '@@ +2,1 @@', got %q", intent.HunkHeader)
	}
	// Context lines
	if len(intent.ContextLines) != 1 || intent.ContextLines[0] != "}" {
		t.Errorf("expected 1 context line '}', got %v", intent.ContextLines)
	}
}

func TestParseIntentionalDiff_MultiLineIntent(t *testing.T) {
	input := `diff --git a/app.js b/app.js
--- a/app.js
+++ b/app.js
@@ -1,3 +1,6 @@
 const app = require('express')();
+  for (let i = 0; i < arr.length; i++) {
+    const profile = await fetchProfile(arr[i].id);
+    results.push(profile);
+  }

¿~~ b/app.js
@@ +2,3 @@
+  for (let i = 0; i < arr.length; i++) {
+    const profile = await fetchProfile(arr[i].id);
+    results.push(profile);
-{¿~ N+1 query — sequential await in loop, use Promise.all() ¿~}
   }
`
	fd, err := ParseIntentionalDiff(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fd.TotalIntents() != 1 {
		t.Fatalf("expected 1 intent, got %d", fd.TotalIntents())
	}
	intent := fd.Files[0].Intents[0]
	if len(intent.AffectedLines) != 3 {
		t.Errorf("expected 3 affected lines, got %d", len(intent.AffectedLines))
	}
	if intent.Symbol != "~" {
		t.Errorf("expected symbol '~', got %q", intent.Symbol)
	}
}

func TestParseIntentionalDiff_MultipleIntentsSameFile(t *testing.T) {
	input := `diff --git a/app.js b/app.js
--- a/app.js
+++ b/app.js
@@ -1,5 +1,7 @@
+  eval(input);
+  for (let i = 0; i < x; i++) { y(); }

¿!! b/app.js
@@ +1,1 @@
+  eval(input);
-{¿! Command injection — never use eval on user input ¿!}

¿~~ b/app.js
@@ +2,1 @@
+  for (let i = 0; i < x; i++) { y(); }
-{¿~ Inefficient loop — use forEach or map ¿~}
`
	fd, err := ParseIntentionalDiff(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fd.Files) != 1 {
		t.Fatalf("expected 1 file (grouped), got %d", len(fd.Files))
	}
	if fd.TotalIntents() != 2 {
		t.Errorf("expected 2 intents, got %d", fd.TotalIntents())
	}
	if fd.Files[0].Intents[0].Symbol != "!" {
		t.Errorf("first intent should be !, got %q", fd.Files[0].Intents[0].Symbol)
	}
	if fd.Files[0].Intents[1].Symbol != "~" {
		t.Errorf("second intent should be ~, got %q", fd.Files[0].Intents[1].Symbol)
	}
}

func TestParseIntentionalDiff_MultipleFiles(t *testing.T) {
	input := `diff --git a/foo.js b/foo.js
--- a/foo.js
+++ b/foo.js
@@ -1,3 +1,3 @@
+  eval(input);
diff --git a/bar.js b/bar.js
--- a/bar.js
+++ b/bar.js
@@ -1,3 +1,3 @@
+  duplicated();

¿!! b/foo.js
@@ +1,1 @@
+  eval(input);
-{¿! Command injection ¿!}

¿== b/bar.js
@@ +1,1 @@
+  duplicated();
-{¿= Same logic in utils.js:10 ¿=}
`
	fd, err := ParseIntentionalDiff(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fd.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(fd.Files))
	}
	if fd.TotalIntents() != 2 {
		t.Errorf("expected 2 total intents, got %d", fd.TotalIntents())
	}
	if fd.Files[0].Path != "foo.js" {
		t.Errorf("expected first file 'foo.js', got %q", fd.Files[0].Path)
	}
	if fd.Files[1].Path != "bar.js" {
		t.Errorf("expected second file 'bar.js', got %q", fd.Files[1].Path)
	}
}

func TestParseIntentionalDiff_NoIntents(t *testing.T) {
	input := `diff --git a/clean.go b/clean.go
--- a/clean.go
+++ b/clean.go
@@ -1,3 +1,4 @@
 package clean
+import "fmt"
 func Clean() {}
`
	fd, err := ParseIntentionalDiff(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fd.TotalIntents() != 0 {
		t.Errorf("expected 0 intents, got %d", fd.TotalIntents())
	}
	if !strings.Contains(fd.RawDiff, "clean.go") {
		t.Error("expected RawDiff to contain the diff content")
	}
}

func TestParseIntentionalDiff_AllSymbolTypes(t *testing.T) {
	input := `diff --git a/test.js b/test.js
--- a/test.js
+++ b/test.js
@@ -1,10 +1,10 @@
+  line1
+  line2
+  line3
+  line4
+  line5
+  line6

¿!! b/test.js
@@ +1,1 @@
+  line1
-{¿! security issue ¿!}

¿~~ b/test.js
@@ +2,1 @@
+  line2
-{¿~ performance issue ¿~}

¿&& b/test.js
@@ +3,1 @@
+  line3
-{¿& coupling issue ¿&}

¿## b/test.js
@@ +4,1 @@
+  line4
-{¿# cohesion issue ¿#}

¿== b/test.js
@@ +5,1 @@
+  line5
-{¿= dry violation ¿=}

¿?? b/test.js
@@ +6,1 @@
+  line6
-{¿? kiss violation ¿?}
`
	fd, err := ParseIntentionalDiff(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fd.TotalIntents() != 6 {
		t.Errorf("expected 6 intents, got %d", fd.TotalIntents())
	}

	expectedSymbols := []string{"!", "~", "&", "#", "=", "?"}
	for i, intent := range fd.Files[0].Intents {
		if intent.Symbol != expectedSymbols[i] {
			t.Errorf("intent %d: expected symbol %q, got %q", i, expectedSymbols[i], intent.Symbol)
		}
	}
}

func TestRenderIntentBlock(t *testing.T) {
	intent := Intent{
		Symbol:        "!",
		FilePath:      "handler.go",
		HunkHeader:    "@@ +12,1 @@",
		AffectedLines: []string{`  db.Exec("SELECT *" + id)`},
		Explanation:   "SQL injection — use parameterized queries",
		ContextLines:  []string{"}"},
	}

	rendered := RenderIntentBlock(intent)

	expected := []string{
		"¿!! b/handler.go",
		"@@ +12,1 @@",
		`+  db.Exec("SELECT *" + id)`,
		"-{¿! SQL injection — use parameterized queries ¿!}",
		" }",
	}
	for _, s := range expected {
		if !strings.Contains(rendered, s) {
			t.Errorf("expected rendered block to contain %q, got:\n%s", s, rendered)
		}
	}
}

func TestRenderFocusedDiff(t *testing.T) {
	fd := &FocusedDiff{
		RawDiff: "diff --git a/f.go b/f.go\n--- a/f.go\n+++ b/f.go\n@@ -1,3 +1,4 @@\n+new line\n",
		Files: []FocusedFile{
			{
				Path: "f.go",
				Intents: []Intent{
					{
						Symbol:        "!",
						FilePath:      "f.go",
						HunkHeader:    "@@ +1,1 @@",
						AffectedLines: []string{"new line"},
						Explanation:   "test issue",
					},
				},
			},
		},
	}

	rendered := RenderFocusedDiff(fd)
	if !strings.Contains(rendered, "diff --git") {
		t.Error("expected raw diff in output")
	}
	if !strings.Contains(rendered, "¿!! b/f.go") {
		t.Error("expected intent block header in output")
	}
	if !strings.Contains(rendered, "-{¿! test issue ¿!}") {
		t.Error("expected intent comment in output")
	}
}

func TestWriteAndReadRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test", "123.intentional.diff")

	original := &FocusedDiff{
		RawDiff: "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1,3 +1,4 @@\n package main\n+  db.Exec(q + id)\n }\n",
		Files: []FocusedFile{
			{
				Path: "main.go",
				Intents: []Intent{
					{
						Symbol:        "!",
						Name:          "Security Risk",
						FilePath:      "main.go",
						HunkHeader:    "@@ +2,1 @@",
						AffectedLines: []string{"  db.Exec(q + id)"},
						Explanation:   "SQL injection — use parameterized queries",
						Severity:      "critical",
					},
				},
			},
		},
	}

	if err := WriteFocusedDiff(path, original); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	loaded, err := ReadFocusedDiff(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if loaded.TotalIntents() != 1 {
		t.Errorf("expected 1 intent after round-trip, got %d", loaded.TotalIntents())
	}

	intent := loaded.Files[0].Intents[0]
	if intent.Symbol != "!" {
		t.Errorf("expected symbol '!', got %q", intent.Symbol)
	}
	if !strings.Contains(intent.Explanation, "SQL injection") {
		t.Errorf("expected explanation to contain 'SQL injection', got %q", intent.Explanation)
	}
	if len(intent.AffectedLines) != 1 {
		t.Errorf("expected 1 affected line, got %d", len(intent.AffectedLines))
	}
}

func TestReadFocusedDiff_NotFound(t *testing.T) {
	_, err := ReadFocusedDiff("/nonexistent/path.intentional.diff")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestTotalIntents(t *testing.T) {
	fd := &FocusedDiff{
		Files: []FocusedFile{
			{Intents: []Intent{{}, {}}},
			{Intents: []Intent{{}}},
			{Intents: nil},
		},
	}
	if fd.TotalIntents() != 3 {
		t.Errorf("expected 3 total intents, got %d", fd.TotalIntents())
	}
}

func TestSymbolToName(t *testing.T) {
	tests := map[string]string{
		"!": "Security Risk",
		"~": "Performance Drag",
		"$": "Resource Cost",
		"&": "Coupling Violation",
		"#": "Cohesion / SOLID Issue",
		"=": "DRY Violation",
		"?": "KISS Violation",
		"X": "Unknown",
	}
	for symbol, expected := range tests {
		if got := SymbolToName(symbol); got != expected {
			t.Errorf("SymbolToName(%q) = %q, want %q", symbol, got, expected)
		}
	}
}

func TestSymbolToSeverity(t *testing.T) {
	if s := SymbolToSeverity("!"); s != "critical" {
		t.Errorf("expected 'critical' for !, got %q", s)
	}
	if s := SymbolToSeverity("~"); s != "warning" {
		t.Errorf("expected 'warning' for ~, got %q", s)
	}
	if s := SymbolToSeverity("#"); s != "info" {
		t.Errorf("expected 'info' for #, got %q", s)
	}
}

func TestParseExampleFile(t *testing.T) {
	data, err := os.ReadFile("../../examples/42.intentional.diff")
	if err != nil {
		t.Skip("example file not found, skipping")
	}

	fd, err := ParseIntentionalDiff(string(data))
	if err != nil {
		t.Fatalf("failed to parse example: %v", err)
	}

	if fd.TotalIntents() != 4 {
		t.Errorf("expected 4 intents in example, got %d", fd.TotalIntents())
	}
	if len(fd.Files) != 2 {
		t.Errorf("expected 2 files in example, got %d", len(fd.Files))
	}
	if !strings.Contains(fd.RawDiff, "diff --git") {
		t.Error("expected RawDiff to contain original diff")
	}

	// Verify intent symbols
	symbols := []string{}
	for _, f := range fd.Files {
		for _, intent := range f.Intents {
			symbols = append(symbols, intent.Symbol)
		}
	}
	expected := []string{"!", "~", "#", "="}
	if len(symbols) != len(expected) {
		t.Fatalf("expected symbols %v, got %v", expected, symbols)
	}
	for i, s := range expected {
		if symbols[i] != s {
			t.Errorf("intent %d: expected symbol %q, got %q", i, s, symbols[i])
		}
	}
}
