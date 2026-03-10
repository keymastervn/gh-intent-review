package ui

import (
	"fmt"
	"os"
	"strings"
	"unicode"

	"golang.org/x/term"
)

// CommentEditor is a minimal multi-line terminal editor for writing PR comments.
//
// Key bindings:
//   - Printable chars                  → insert at cursor
//   - Backspace                        → delete char before cursor (merges lines at start of line)
//   - Option+Backspace / Ctrl+W        → delete word before cursor
//   - Arrow Left / Right               → move cursor within line (wraps between lines)
//   - Option+Left / Option+Right       → move cursor by word
//     macOS Terminal: ESC+b / ESC+f
//     iTerm2/WezTerm: CSI 1;3D / CSI 1;3C
//   - Arrow Up on row 0                → load AI suggestion (replaces buffer)
//   - Arrow Up on other rows           → move cursor up
//   - Arrow Down                       → move cursor down (no-op at last line)
//   - Shift+Enter                      → insert newline (via Kitty/modifyOtherKeys protocol)
//   - Alt+Enter (Option+Enter)         → insert newline (fallback: ESC+CR for macOS Terminal)
//   - Enter / Ctrl+D                   → submit comment
//   - Ctrl+C                           → cancel
//
// Redraw uses \033[{n}A to move the cursor up exactly n TERMINAL rows (not logical
// lines), computed via term.GetSize so line-wrapping is accounted for correctly.
type CommentEditor struct {
	lines      []string
	row        int
	col        int // rune index within current line
	suggestion string
	drawn      int // terminal rows drawn in the last render() call
}

// hintText is the visible content of the hint bar (no ANSI codes).
const hintText = "  └─  ↑ load suggestion · Shift+Enter new line · Enter submit · Ctrl+C cancel"

// NewCommentEditor creates a comment editor. The suggestion is loaded when ↑ is pressed on row 0.
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
		var line string
		fmt.Scanln(&line)
		return line, line != ""
	}
	defer term.Restore(fd, oldState)

	// Enable extended keyboard protocols so Shift+Enter is distinguishable from Enter.
	// Terminals that don't support a protocol silently ignore its enable sequence.
	//   Kitty keyboard protocol (flag=1: disambiguate) → Shift+Enter sends \x1b[13;2u
	//   XTerm modifyOtherKeys level 2                  → Shift+Enter sends \x1b[27;2;13~
	fmt.Print("\x1b[>4;2m") // modifyOtherKeys level 2
	fmt.Print("\x1b[>1u")   // Kitty: push flag=1 (disambiguate escape codes)
	defer func() {
		fmt.Print("\x1b[>4m") // reset modifyOtherKeys
		fmt.Print("\x1b[<u")  // Kitty: pop from stack
	}()

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
		// ── Submit / cancel ────────────────────────────────────────────────────
		case n == 1 && seq[0] == '\r': // Enter → submit
			e.erase()
			return strings.Join(e.lines, "\n"), true

		case n == 1 && seq[0] == 0x04: // Ctrl+D → submit
			e.erase()
			return strings.Join(e.lines, "\n"), true

		case n == 1 && seq[0] == 0x03: // Ctrl+C → cancel
			e.erase()
			return "", false

		// ── Newline insertion ──────────────────────────────────────────────────
		// Alt/Option+Enter: ESC + CR (2 bytes) — macOS Terminal & iTerm2 (Option key = Esc+)
		case n == 2 && seq[0] == 0x1B && seq[1] == '\r':
			e.insertNewline()

		// Shift+Enter in CSI-u terminals (kitty, WezTerm with CSI-u enabled)
		// Note: macOS Terminal/iTerm2 send bare \r for Shift+Enter — same as Enter, undetectable.

		// ── Delete ─────────────────────────────────────────────────────────────
		case n == 1 && (seq[0] == 0x7F || seq[0] == 0x08): // Backspace
			e.backspace()

		case n == 1 && seq[0] == 0x17: // Ctrl+W → delete word backward
			e.deleteWordBackward()

		case n == 2 && seq[0] == 0x1B && seq[1] == 0x7F: // Option+Backspace (ESC+DEL)
			e.deleteWordBackward()

		// ── Word navigation — macOS Terminal default (ESC+b / ESC+f) ──────────
		case n == 2 && seq[0] == 0x1B && seq[1] == 'b':
			e.moveWordLeft()

		case n == 2 && seq[0] == 0x1B && seq[1] == 'f':
			e.moveWordRight()

		// ── CSI sequences (arrows, word nav, Shift+Enter CSI-u) ───────────────
		case n >= 3 && seq[0] == 0x1B && seq[1] == '[':
			rest := string(seq[2:n])
			switch rest {
			case "A": // Arrow Up
				if e.row > 0 {
					e.row--
					e.clampCol()
				} else {
					e.loadSuggestion()
				}

			case "B": // Arrow Down — no-op at last line
				if e.row < len(e.lines)-1 {
					e.row++
					e.clampCol()
				}

			case "C": // Arrow Right
				runes := []rune(e.lines[e.row])
				if e.col < len(runes) {
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

			case "1;3C", "1;9C": // Option+Right (iTerm2, WezTerm, Alacritty)
				e.moveWordRight()
			case "1;3D", "1;9D": // Option+Left
				e.moveWordLeft()

			case "13;2u": // Shift+Enter — Kitty keyboard protocol
				e.insertNewline()
			case "27;2;13~": // Shift+Enter — XTerm modifyOtherKeys level 2
				e.insertNewline()
			}

		// ── Printable ASCII ────────────────────────────────────────────────────
		case n == 1 && seq[0] >= 0x20 && seq[0] < 0x7F:
			e.insertRune(rune(seq[0]))

		// ── UTF-8 multi-byte (é, 中, emoji, …) ────────────────────────────────
		default:
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

// ── Editing operations ────────────────────────────────────────────────────────

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
	e.lines[e.row] = string(runes[:e.col])
	after := string(runes[e.col:])
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

func (e *CommentEditor) deleteWordBackward() {
	if e.col > 0 {
		runes := []rune(e.lines[e.row])
		newCol := wordBoundaryLeft(runes, e.col)
		updated := make([]rune, len(runes)-(e.col-newCol))
		copy(updated, runes[:newCol])
		copy(updated[newCol:], runes[e.col:])
		e.lines[e.row] = string(updated)
		e.col = newCol
	} else if e.row > 0 {
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

func (e *CommentEditor) moveWordLeft() {
	if e.col > 0 {
		e.col = wordBoundaryLeft([]rune(e.lines[e.row]), e.col)
	} else if e.row > 0 {
		e.row--
		e.col = len([]rune(e.lines[e.row]))
	}
}

func (e *CommentEditor) moveWordRight() {
	runes := []rune(e.lines[e.row])
	if e.col < len(runes) {
		e.col = wordBoundaryRight(runes, e.col)
	} else if e.row < len(e.lines)-1 {
		e.row++
		e.col = 0
	}
}

func (e *CommentEditor) loadSuggestion() {
	if e.suggestion == "" {
		return
	}
	lines := strings.Split(e.suggestion, "\n")
	// Trim trailing empty element from a suggestion that ends with \n
	for len(lines) > 1 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	e.lines = lines
	e.row = len(e.lines) - 1
	e.col = len([]rune(e.lines[e.row]))
}

func (e *CommentEditor) clampCol() {
	if max := len([]rune(e.lines[e.row])); e.col > max {
		e.col = max
	}
}

// ── Word boundary helpers ─────────────────────────────────────────────────────

func wordBoundaryLeft(runes []rune, col int) int {
	i := col
	for i > 0 && unicode.IsSpace(runes[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(runes[i-1]) {
		i--
	}
	return i
}

func wordBoundaryRight(runes []rune, col int) int {
	n, i := len(runes), col
	for i < n && unicode.IsSpace(runes[i]) {
		i++
	}
	for i < n && !unicode.IsSpace(runes[i]) {
		i++
	}
	return i
}

// ── Rendering ─────────────────────────────────────────────────────────────────

// render draws the editor in-place. It moves the cursor up by the number of
// TERMINAL ROWS (not logical lines) that the previous render occupied, computed
// using the current terminal width so line-wrapping is handled correctly.
// All output uses \r\n because raw mode disables CR/LF output translation.
func (e *CommentEditor) render() {
	tw := terminalWidth()

	// Move cursor up past the previous render and clear everything below.
	if e.drawn > 0 {
		fmt.Printf("\033[%dA\033[J", e.drawn)
	}

	rows := 0
	for i, line := range e.lines {
		runes := []rune(line)
		if i == e.row {
			before := string(runes[:e.col])
			after := string(runes[e.col:])
			// Cursor shown as an inverted-video space.
			fmt.Printf("  \033[2m│\033[0m %s\033[7m \033[0m%s\r\n", before, after)
			// Visual width: "  │ " (4) + content runes + cursor space (1)
			rows += wrapRows(4+len(runes)+1, tw)
		} else {
			fmt.Printf("  \033[2m│\033[0m %s\r\n", line)
			rows += wrapRows(4+len(runes), tw)
		}
	}

	fmt.Printf("\033[2m%s\033[0m\r\n", hintText)
	rows += wrapRows(len([]rune(hintText)), tw)

	e.drawn = rows
}

// erase clears the editor area drawn by the last render() call.
func (e *CommentEditor) erase() {
	if e.drawn > 0 {
		fmt.Printf("\033[%dA\033[J", e.drawn)
		e.drawn = 0
	}
}

// terminalWidth returns the current terminal column count, defaulting to 80.
func terminalWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

// wrapRows returns how many terminal rows a line of visWidth visible characters
// occupies in a terminal that is tw columns wide.
func wrapRows(visWidth, tw int) int {
	if visWidth <= 0 || tw <= 0 {
		return 1
	}
	r := (visWidth + tw - 1) / tw
	if r < 1 {
		return 1
	}
	return r
}
