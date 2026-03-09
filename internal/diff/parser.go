package diff

import (
	"bufio"
	"fmt"
	"strings"
)

// FileDiff represents the diff for a single file.
type FileDiff struct {
	OldName  string
	NewName  string
	Hunks    []Hunk
	IsBinary bool
}

// Hunk represents a section of a diff.
type Hunk struct {
	Header string
	Lines  []DiffLine
}

// DiffLine represents a single line in a diff.
type DiffLine struct {
	Type    LineType
	Content string
	OldNum  int
	NewNum  int
}

// LineType indicates the type of a diff line.
type LineType int

const (
	LineContext LineType = iota // unchanged line
	LineAdded                   // + line
	LineRemoved                 // - line
)

// ParseUnifiedDiff parses a unified diff string into structured file diffs.
func ParseUnifiedDiff(raw string) ([]FileDiff, error) {
	var files []FileDiff
	var current *FileDiff
	var currentHunk *Hunk

	scanner := bufio.NewScanner(strings.NewReader(raw))
	oldNum, newNum := 0, 0

	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, "diff --git"):
			if current != nil {
				if currentHunk != nil {
					current.Hunks = append(current.Hunks, *currentHunk)
				}
				files = append(files, *current)
			}
			current = &FileDiff{}
			currentHunk = nil

		case strings.HasPrefix(line, "--- "):
			if current != nil {
				current.OldName = strings.TrimPrefix(line, "--- ")
				if strings.HasPrefix(current.OldName, "a/") {
					current.OldName = current.OldName[2:]
				}
			}

		case strings.HasPrefix(line, "+++ "):
			if current != nil {
				current.NewName = strings.TrimPrefix(line, "+++ ")
				if strings.HasPrefix(current.NewName, "b/") {
					current.NewName = current.NewName[2:]
				}
			}

		case strings.HasPrefix(line, "@@"):
			if current != nil && currentHunk != nil {
				current.Hunks = append(current.Hunks, *currentHunk)
			}
			currentHunk = &Hunk{Header: line}
			// Parse @@ -old,count +new,count @@
			fmt.Sscanf(line, "@@ -%d", &oldNum)
			fmt.Sscanf(line, "@@ -%*d,%*d +%d", &newNum)

		case strings.HasPrefix(line, "Binary"):
			if current != nil {
				current.IsBinary = true
			}

		default:
			if currentHunk == nil {
				continue
			}

			dl := DiffLine{Content: line}
			switch {
			case strings.HasPrefix(line, "+"):
				dl.Type = LineAdded
				dl.Content = line[1:]
				dl.NewNum = newNum
				newNum++
			case strings.HasPrefix(line, "-"):
				dl.Type = LineRemoved
				dl.Content = line[1:]
				dl.OldNum = oldNum
				oldNum++
			default:
				dl.Type = LineContext
				if len(line) > 0 {
					dl.Content = line[1:] // strip leading space
				}
				dl.OldNum = oldNum
				dl.NewNum = newNum
				oldNum++
				newNum++
			}
			currentHunk.Lines = append(currentHunk.Lines, dl)
		}
	}

	// Don't forget the last file
	if current != nil {
		if currentHunk != nil {
			current.Hunks = append(current.Hunks, *currentHunk)
		}
		files = append(files, *current)
	}

	return files, scanner.Err()
}

// String renders a FileDiff back to a readable string (for context).
func (f *FileDiff) String() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("--- %s\n+++ %s\n", f.OldName, f.NewName))
	for _, h := range f.Hunks {
		b.WriteString(h.Header + "\n")
		for _, l := range h.Lines {
			switch l.Type {
			case LineAdded:
				b.WriteString("+" + l.Content + "\n")
			case LineRemoved:
				b.WriteString("-" + l.Content + "\n")
			default:
				b.WriteString(" " + l.Content + "\n")
			}
		}
	}
	return b.String()
}
