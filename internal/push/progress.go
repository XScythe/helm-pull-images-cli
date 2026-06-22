package push

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"golang.org/x/term"
)

var isTerminalWriter = func(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

type transferProgress struct {
	mu        sync.Mutex
	w         io.Writer
	label     string
	total     int
	completed int
	active    []string
	terminal  bool
	lastLen   int
	lastLines int
	width     int
}

func newTransferProgress(w io.Writer, label string, total int) *transferProgress {
	if w == nil {
		w = io.Discard
	}
	terminal := isTerminalWriter(w)
	width := 0
	if terminal {
		width = terminalWidth(w)
	}
	return &transferProgress{
		w:        w,
		label:    label,
		total:    total,
		terminal: terminal,
		width:    width,
	}
}

func (p *transferProgress) Begin(item string) {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.active = append(p.active, item)
	p.renderLocked(item, false)
}

func (p *transferProgress) End(item string) {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for i, active := range p.active {
		if active == item {
			p.active = append(p.active[:i], p.active[i+1:]...)
			break
		}
	}
	p.completed++
	p.renderLocked(item, true)
}

func (p *transferProgress) Finish() {
	if p == nil || p.total <= 0 || !p.terminal {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.lastLines > 0 {
		fmt.Fprintln(p.w)
	}
	p.lastLen = 0
	p.lastLines = 0
}

func (p *transferProgress) renderLocked(item string, finished bool) {
	if p.total <= 0 {
		return
	}
	if !p.terminal {
		if !finished {
			fmt.Fprintf(p.w, "%s %d/%d: %s\n", p.label, p.completed+len(p.active), p.total, normalizeDisplayImage(item))
		}
		return
	}

	lines := p.formatLinesLocked()
	if p.width > 0 {
		for i, line := range lines {
			lines[i] = truncateForWidth(line, p.width-1)
		}
	}
	if p.lastLines > 0 {
		clearProgressBlock(p.w, p.lastLines)
	}
	for i, line := range lines {
		if i > 0 {
			fmt.Fprint(p.w, "\n")
		}
		fmt.Fprint(p.w, line)
	}
	p.lastLines = len(lines)
	if len(lines) > 0 {
		p.lastLen = len(lines[len(lines)-1])
	}
}

func (p *transferProgress) formatLinesLocked() []string {
	barWidth := 18
	filled := 0
	if p.total > 0 {
		filled = (p.completed * barWidth) / p.total
	}
	if filled > barWidth {
		filled = barWidth
	}

	bar := strings.Repeat("=", filled)
	if filled < barWidth {
		bar += ">"
		bar += strings.Repeat("-", barWidth-filled-1)
	}

	lines := []string{fmt.Sprintf("%s [%s] %d/%d", p.label, bar, p.completed, p.total)}
	active := summarizeActive(p.active)
	if len(active) > 0 {
		lines = append(lines, active...)
	}
	return lines
}

func summarizeActive(active []string) []string {
	if len(active) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(active))
	for _, item := range active {
		normalized = append(normalized, normalizeDisplayImage(item))
	}
	return normalized
}

func statusWriter(status ...io.Writer) io.Writer {
	if len(status) > 0 && status[0] != nil {
		return status[0]
	}
	return io.Discard
}

func normalizeDisplayImage(image string) string {
	switch {
	case strings.HasPrefix(image, "docker.io/"):
		return strings.TrimPrefix(image, "docker.io/")
	case strings.HasPrefix(image, "index.docker.io/"):
		return strings.TrimPrefix(image, "index.docker.io/")
	default:
		return image
	}
}

func terminalWidth(w io.Writer) int {
	f, ok := w.(*os.File)
	if !ok {
		return 0
	}
	width, _, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return 0
	}
	return width
}

func truncateForWidth(line string, width int) string {
	if width <= 0 {
		return line
	}
	if len(line) <= width {
		return line
	}
	return line[:width]
}

func clearProgressBlock(w io.Writer, lines int) {
	if lines <= 0 {
		return
	}
	fmt.Fprint(w, "\r\x1b[2K")
	for i := 1; i < lines; i++ {
		fmt.Fprint(w, "\x1b[1A\r\x1b[2K")
	}
}
