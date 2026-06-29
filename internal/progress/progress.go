package progress

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

const minRenderInterval = 80 * time.Millisecond

// IsTerminalWriter reports whether w is a terminal.
var IsTerminalWriter = func(w io.Writer) bool {
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

// IsTerminalReader reports whether r is a terminal.
func IsTerminalReader(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// StatusWriter returns the provided status writer or io.Discard.
func StatusWriter(status ...io.Writer) io.Writer {
	if len(status) > 0 && status[0] != nil {
		return status[0]
	}
	return io.Discard
}

// NormalizeDisplayImage removes common Docker Hub registry prefixes for display.
func NormalizeDisplayImage(image string) string {
	switch {
	case strings.HasPrefix(image, "docker.io/"):
		return strings.TrimPrefix(image, "docker.io/")
	case strings.HasPrefix(image, "index.docker.io/"):
		return strings.TrimPrefix(image, "index.docker.io/")
	default:
		return image
	}
}

// TerminalWidth returns the width of the terminal backing w, or 0.
func TerminalWidth(w io.Writer) int {
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

// TruncateForWidth truncates line to width bytes when width is positive.
func TruncateForWidth(line string, width int) string {
	if width <= 0 {
		return line
	}
	if len(line) <= width {
		return line
	}
	return line[:width]
}

// HumanizeBytes renders n in binary units with compact formatting.
func HumanizeBytes(n int64) string {
	if n <= 0 {
		return "0B"
	}

	units := []string{"B", "KB", "MB", "GB", "TB", "PB", "EB"}
	value := float64(n)
	unit := 0
	for unit < len(units)-1 && value >= 1024 {
		value /= 1024
		unit++
	}

	if unit <= 2 {
		return fmt.Sprintf("%d%s", int64(value), units[unit])
	}
	return fmt.Sprintf("%.1f%s", value, units[unit])
}

type imageState struct {
	complete int64
	total    int64
	stage    string
}

type Progress struct {
	mu        sync.Mutex
	w         io.Writer
	label     string
	total     int
	completed int
	active    []string
	states    map[string]*imageState
	terminal  bool
	lastLen    int
	lastLines  int
	width      int
	lastRender time.Time
}

// New creates a new terminal-aware progress renderer.
func New(w io.Writer, label string, total int) *Progress {
	if w == nil {
		w = io.Discard
	}
	terminal := IsTerminalWriter(w)
	width := 0
	if terminal {
		width = TerminalWidth(w)
	}
	return &Progress{
		w:        w,
		label:    label,
		total:    total,
		states:   make(map[string]*imageState),
		terminal: terminal,
		width:    width,
	}
}

// Begin marks item as active and renders progress.
func (p *Progress) Begin(item string) {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	st := p.ensureStateLocked(item)
	st.stage = "fetching"
	p.addActiveLocked(item)
	p.renderLocked(item, false)
}

// Update replaces the tracked state for item and renders progress.
func (p *Progress) Update(item string, complete, total int64, stage string) {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	st := p.ensureStateLocked(item)
	st.complete = complete
	st.total = total
	st.stage = stage
	p.renderLocked(item, false)
}

// End marks item as complete and renders progress.
func (p *Progress) End(item string) {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	st := p.ensureStateLocked(item)
	switch {
	case st.total > 0 && st.complete == 0:
		st.complete = st.total
		st.stage = "cached"
	case st.total > 0 && st.complete < st.total:
		st.complete = st.total
		st.stage = "done"
	default:
		st.stage = "done"
	}
	p.removeActiveLocked(item)
	p.completed++

	if !p.terminal {
		p.renderLocked(item, true)
		return
	}
	if p.total <= 0 {
		return
	}

	if p.lastLines > 0 {
		clearProgressBlock(p.w, p.lastLines)
		p.lastLines = 0
	}
	summary := fmt.Sprintf("%s %s %s", NormalizeDisplayImage(item), HumanizeBytes(st.total), st.stage)
	if p.width > 0 {
		summary = TruncateForWidth(summary, p.width-1)
	}
	_, _ = fmt.Fprintln(p.w, summary)
	p.drawLiveBlockLocked()
}

func (p *Progress) drawLiveBlockLocked() {
	lines := p.formatLinesLocked()
	if p.width > 0 {
		for i, line := range lines {
			lines[i] = TruncateForWidth(line, p.width-1)
		}
	}
	for i, line := range lines {
		if i > 0 {
			_, _ = fmt.Fprint(p.w, "\n")
		}
		_, _ = fmt.Fprint(p.w, line)
	}
	p.lastLines = len(lines)
	if len(lines) > 0 {
		p.lastLen = len(lines[len(lines)-1])
	}
	p.lastRender = time.Now()
}

// Finish clears any terminal progress block.
func (p *Progress) Finish() {
	if p == nil || p.total <= 0 || !p.terminal {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.lastLines > 0 {
		_, _ = fmt.Fprintln(p.w)
	}
	p.lastLen = 0
	p.lastLines = 0
}

func (p *Progress) renderLocked(item string, finished bool) {
	if p.total <= 0 {
		return
	}
	if !p.terminal {
		if !finished {
			return
		}
		st := p.stateForLocked(item)
		_, _ = fmt.Fprintf(
			p.w,
			"%s %d/%d: %s %s/%s %s\n",
			p.label,
			p.completed,
			p.total,
			NormalizeDisplayImage(item),
			HumanizeBytes(st.complete),
			HumanizeBytes(st.total),
			st.stage,
		)
		return
	}

	if !finished && time.Since(p.lastRender) < minRenderInterval {
		return
	}
	p.lastRender = time.Now()

	lines := p.formatLinesLocked()
	if p.width > 0 {
		for i, line := range lines {
			lines[i] = TruncateForWidth(line, p.width-1)
		}
	}
	if p.lastLines > 0 {
		clearProgressBlock(p.w, p.lastLines)
	}
	for i, line := range lines {
		if i > 0 {
			_, _ = fmt.Fprint(p.w, "\n")
		}
		_, _ = fmt.Fprint(p.w, line)
	}
	p.lastLines = len(lines)
	if len(lines) > 0 {
		p.lastLen = len(lines[len(lines)-1])
	}
}

func (p *Progress) formatLinesLocked() []string {
	barWidth := 18
	lines := []string{
		fmt.Sprintf(
			"%s [%s] %d/%d",
			p.label,
			renderBar(int64(p.completed), int64(p.total), barWidth),
			p.completed,
			p.total,
		),
	}
	for _, item := range p.active {
		st := p.stateForLocked(item)
		lines = append(lines, fmt.Sprintf(
			"%s [%s] %s/%s %s",
			NormalizeDisplayImage(item),
			renderBar(st.complete, st.total, barWidth),
			HumanizeBytes(st.complete),
			HumanizeBytes(st.total),
			st.stage,
		))
	}
	return lines
}

func renderBar(complete, total int64, width int) string {
	if width <= 0 {
		return ""
	}
	if total <= 0 {
		return strings.Repeat("-", width)
	}
	filled := int((complete * int64(width)) / total)
	if filled < 0 {
		filled = 0
	}
	if filled >= width {
		return strings.Repeat("=", width)
	}
	return strings.Repeat("=", filled) + ">" + strings.Repeat("-", width-filled-1)
}

func (p *Progress) ensureStateLocked(item string) *imageState {
	if p.states == nil {
		p.states = make(map[string]*imageState)
	}
	st := p.states[item]
	if st == nil {
		st = &imageState{}
		p.states[item] = st
	}
	return st
}

func (p *Progress) stateForLocked(item string) *imageState {
	if p == nil {
		return &imageState{}
	}
	st := p.states[item]
	if st == nil {
		return &imageState{}
	}
	return st
}

func (p *Progress) addActiveLocked(item string) {
	for _, active := range p.active {
		if active == item {
			return
		}
	}
	p.active = append(p.active, item)
}

func (p *Progress) removeActiveLocked(item string) {
	for i, active := range p.active {
		if active == item {
			p.active = append(p.active[:i], p.active[i+1:]...)
			return
		}
	}
}

func clearProgressBlock(w io.Writer, lines int) {
	if lines <= 0 {
		return
	}
	_, _ = fmt.Fprint(w, "\r\x1b[2K")
	for i := 1; i < lines; i++ {
		_, _ = fmt.Fprint(w, "\x1b[1A\r\x1b[2K")
	}
}
