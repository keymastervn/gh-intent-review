package ui

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// CommentEditor is a minimal multi-line terminal editor for writing PR comments.
//
// Key bindings:
//   - Printable chars     → insert at cursor
//   - Backspace           → delete character before cursor (merges lines at start of line)
//   - Arrow Left / Right  → move cursor within line (wraps between lines)
//   - Arrow Up            → if on first line, load suggestion; otherwise move cursor up
//   - Arrow Down          → move cursor down
//   - Ctrl+J (^J)         → insert newline (new line in comment)
//   - Shift+Enter         → insert newline (CSI-u capable terminals: kitty, WezTerm)
//   - Enter               → submit comment
//   - Ctrl+D              → submit comment
//   - Ctrl+C              → cancel (returns submitted=false)
type CommentEditor struct {
	lines      []string
	row        int
	col        int // rune index within current line
	suggestion string
	drawn      int // lines rendered in last render() call — used for clear-and-redraw
}

// NewCommentEditor creates a comment editor. suggestion is loaded when the user presses ↑ on the first line.
func NewCommentEditor(suggestion string) *CommentEditor {
	return &CommentEditor{
		lines:      []string{""},
		suggestion: suggestion,
	}
}

// Run enters raw terminal mode and presents the editor.
// Returns (text, true) on submit, ("", false) on cancel.
func (e *CommentEditor) Run() (text string, submitted bool) {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		// Non-TTY fallback: read a single line from the buffered reader
		var line string
		fmt.Scanln(&line)
		return line, line != ""
	}
	defer term.Restore(fd, oldState)

	e.render()

	buf := make([]byte, 32)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			e.erase()
			return "", false
		}
		seq := buf[:n]

		switch {
		case n == 1 && seq[0] == '\r': // Enter → submit
			e.erase()
			return strings.Join(e.lines, "\n"), true

		case n == 1 && seq[0] == 0x0A: // Ctrl+J → new line
			e.insertNewline()

		case n == 1 && seq[0] == 0x03: // Ctrl+C → cancel
			e.erase()
			return "", false

		case n == 1 && seq[0] == 0x04: // Ctrl+D → submit
			e.erase()
			return strings.Join(e.lines, "\n"), true

		case n == 1 && (seq[0] == 0x7F || seq[0] == 0x08): // Backspace
			e.backspace()

		case n >= 3 && seq[0] == 0x1B && seq[1] == '[': // CSI escape sequences
			rest := string(seq[2:n])
			switch rest {
			case "A": // Arrow Up
				if e.row == 0 {
					e.loadSuggestion()
				} else {
					e.row--
					e.clampCol()
				}
			case "B": // Arrow Down
				if e.row < len(e.lines)-1 {
					e.row++
					e.clampCol()
				}
			case "C": // Arrow Right
				lineRunes := []rune(e.lines[e.row])
				if e.col < len(lineRunes) {
					e.col++
				} else if e.row < len(e.lines)-1 {
					e.row++
					e.col = 0
				}
			case "D": // Arrow Left
				if e.col > 0 {
					e.col--
				} else if e.row > 0 {
					e.row--
					e.col = len([]rune(e.lines[e.row]))
				}
			case "13;2u": // Shift+Enter (CSI-u: kitty, WezTerm)
				e.insertNewline()
			}

		case n == 1 && seq[0] >= 0x20 && seq[0] < 0x7F: // printable ASCII
			e.insertRune(rune(seq[0]))

		default:
			// UTF-8 multi-byte character (e.g. é, 中, emoji)
			if n > 1 && seq[0] >= 0xC0 {
				for _, r := range string(seq[:n]) {
					if r >= 0x20 {
						e.insertRune(r)
					}
				}
			}
		}

		e.render()
	}
}

func (e *CommentEditor) insertRune(r rune) {
	runes := []rune(e.lines[e.row])
	updated := make([]rune, len(runes)+1)
	copy(updated, runes[:e.col])
	updated[e.col] = r
	copy(updated[e.col+1:], runes[e.col:])
	e.lines[e.row] = string(updated)
	e.col++
}

func (e *CommentEditor) insertNewline() {
	runes := []rune(e.lines[e.row])
	before := string(runes[:e.col])
	after := string(runes[e.col:])
	e.lines[e.row] = before

	newLines := make([]string, len(e.lines)+1)
	copy(newLines, e.lines[:e.row+1])
	newLines[e.row+1] = after
	copy(newLines[e.row+2:], e.lines[e.row+1:])
	e.lines = newLines
	e.row++
	e.col = 0
}

func (e *CommentEditor) backspace() {
	if e.col > 0 {
		runes := []rune(e.lines[e.row])
		updated := make([]rune, len(runes)-1)
		copy(updated, runes[:e.col-1])
		copy(updated[e.col-1:], runes[e.col:])
		e.lines[e.row] = string(updated)
		e.col--
	} else if e.row > 0 {
		// Merge current line into previous
		prev := e.lines[e.row-1]
		e.col = len([]rune(prev))
		e.lines[e.row-1] = prev + e.lines[e.row]
		newLines := make([]string, len(e.lines)-1)
		copy(newLines, e.lines[:e.row])
		copy(newLines[e.row:], e.lines[e.row+1:])
		e.lines = newLines
		e.row--
	}
}

func (e *CommentEditor) loadSuggestion() {
	if e.suggestion == "" {
		return
	}
	e.lines = strings.Split(e.suggestion, "\n")
	e.row = len(e.lines) - 1
	e.col = len([]rune(e.lines[e.row]))
}

func (e *CommentEditor) clampCol() {
	max := len([]rune(e.lines[e.row]))
	if e.col > max {
		e.col = max
	}
}

// render draws the editor, replacing the previous render in-place.
// Uses \r\n because raw mode does not translate \n to \r\n.
func (e *CommentEditor) render() {
	e.erase()

	count := 0
	for i, line := range e.lines {
		runes := []rune(line)
		if i == e.row {
			before := string(runes[:e.col])
			after := string(runes[e.col:])
			// Block cursor shown as inverted space
			fmt.Printf("  \033[2m│\033[0m %s\033[7m \033[0m%s\r\n", before, after)
		} else {
			fmt.Printf("  \033[2m│\033[0m %s\r\n", line)
		}
		count++
	}

	// Hint bar
	fmt.Printf("  \033[2m└─  ↑ load suggestion · Ctrl+J new line · Enter submit · Ctrl+C cancel\033[0m\r\n")
	count++

	e.drawn = count
}

// erase moves the cursor up and clears all lines drawn by the last render() call.
func (e *CommentEditor) erase() {
	if e.drawn > 0 {
		fmt.Printf("\033[%dA\033[J", e.drawn)
		e.drawn = 0
	}
}
