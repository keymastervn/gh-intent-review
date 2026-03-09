package diff

import (
	"testing"
)

func TestParseUnifiedDiff_SingleFile(t *testing.T) {
	raw := `diff --git a/main.go b/main.go
index abc1234..def5678 100644
--- a/main.go
+++ b/main.go
@@ -1,5 +1,6 @@
 package main

 func main() {
-	fmt.Println("hello")
+	fmt.Println("hello world")
+	fmt.Println("goodbye")
 }
`
	files, err := ParseUnifiedDiff(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	f := files[0]
	if f.OldName != "main.go" {
		t.Errorf("expected OldName 'main.go', got %q", f.OldName)
	}
	if f.NewName != "main.go" {
		t.Errorf("expected NewName 'main.go', got %q", f.NewName)
	}
	if len(f.Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(f.Hunks))
	}

	hunk := f.Hunks[0]
	var added, removed, context int
	for _, l := range hunk.Lines {
		switch l.Type {
		case LineAdded:
			added++
		case LineRemoved:
			removed++
		case LineContext:
			context++
		}
	}
	if added != 2 {
		t.Errorf("expected 2 added lines, got %d", added)
	}
	if removed != 1 {
		t.Errorf("expected 1 removed line, got %d", removed)
	}
	if context != 4 {
		t.Errorf("expected 4 context lines, got %d", context)
	}
}

func TestParseUnifiedDiff_MultipleFiles(t *testing.T) {
	raw := `diff --git a/foo.go b/foo.go
--- a/foo.go
+++ b/foo.go
@@ -1,3 +1,3 @@
 package foo
-var x = 1
+var x = 2
diff --git a/bar.go b/bar.go
--- a/bar.go
+++ b/bar.go
@@ -1,3 +1,4 @@
 package bar
+import "fmt"
 func Bar() {}
`
	files, err := ParseUnifiedDiff(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if files[0].NewName != "foo.go" {
		t.Errorf("expected first file 'foo.go', got %q", files[0].NewName)
	}
	if files[1].NewName != "bar.go" {
		t.Errorf("expected second file 'bar.go', got %q", files[1].NewName)
	}
}

func TestParseUnifiedDiff_BinaryFile(t *testing.T) {
	raw := `diff --git a/image.png b/image.png
Binary files differ
`
	files, err := ParseUnifiedDiff(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if !files[0].IsBinary {
		t.Error("expected IsBinary=true")
	}
}

func TestParseUnifiedDiff_Empty(t *testing.T) {
	files, err := ParseUnifiedDiff("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestFileDiff_String(t *testing.T) {
	fd := FileDiff{
		OldName: "test.go",
		NewName: "test.go",
		Hunks: []Hunk{
			{
				Header: "@@ -1,3 +1,3 @@",
				Lines: []DiffLine{
					{Type: LineContext, Content: "package main"},
					{Type: LineRemoved, Content: "old line"},
					{Type: LineAdded, Content: "new line"},
				},
			},
		},
	}

	s := fd.String()
	if s == "" {
		t.Error("expected non-empty string")
	}
}
