package push

import (
	"bufio"
	"fmt"
	"helm-deep-pack/internal/pushspec"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

const (
	ansiReset   = "\x1b[0m"
	ansiBold    = "\x1b[1m"
	ansiDim     = "\x1b[2m"
	ansiRed     = "\x1b[31m"
	ansiGreen   = "\x1b[32m"
	ansiYellow  = "\x1b[33m"
	ansiMagenta = "\x1b[35m"
	ansiCyan    = "\x1b[36m"
)

type selectModel struct {
	items    []classifiedImage
	registry string
	cursor   int
	checked  []bool
	top      int
	height   int
}

func newSelectModel(items []classifiedImage, height int, registry string) *selectModel {
	if height < 1 {
		if len(items) > 0 {
			height = len(items)
		} else {
			height = 1
		}
	}
	return &selectModel{
		items:    items,
		registry: strings.TrimRight(registry, "/"),
		cursor:   0,
		checked:  make([]bool, len(items)),
		top:      0,
		height:   height,
	}
}

func (m *selectModel) moveUp() {
	if m.cursor > 0 {
		m.cursor--
	}
	if m.cursor < m.top {
		m.top = m.cursor
	}
}

func (m *selectModel) moveDown() {
	if m.cursor < len(m.items)-1 {
		m.cursor++
	}
	if m.cursor >= m.top+m.height {
		m.top = m.cursor - m.height + 1
	}
}

func (m *selectModel) toggle() {
	if len(m.items) > 0 {
		m.checked[m.cursor] = !m.checked[m.cursor]
	}
}

func (m *selectModel) toggleAll() {
	allChecked := true
	for _, c := range m.checked {
		if !c {
			allChecked = false
			break
		}
	}
	for i := range m.checked {
		m.checked[i] = !allChecked
	}
}

func (m *selectModel) selectedSpecs() []pushspec.ArchiveSpec {
	var result []pushspec.ArchiveSpec
	for i, checked := range m.checked {
		if checked {
			result = append(result, m.items[i].Spec)
		}
	}
	return result
}

func (m *selectModel) selectedCount() int {
	count := 0
	for _, c := range m.checked {
		if c {
			count++
		}
	}
	return count
}

func (m *selectModel) render() []string {
	lines := make([]string, 0)

	lines = append(lines, "Select images to push (↑/↓ move · space toggle · a all · enter confirm · esc cancel)")

	end := m.top + m.height
	if end > len(m.items) {
		end = len(m.items)
	}
	for i := m.top; i < end; i++ {
		prefix := "  "
		if i == m.cursor {
			prefix = "❯ "
		}
		checkbox := "◯ "
		if m.checked[i] {
			checkbox = "◉ "
		}
		target := m.items[i].Spec.Target
		if m.registry != "" {
			target = m.registry + "/" + target
		}
		body := normalizeDisplayImage(m.items[i].Spec.Image) + " → " + target
		status := ""
		switch m.items[i].Status {
		case statusPushable:
			status = "  [missing]"
		case statusMirrored:
			status = "  [exists]"
		case statusConflict:
			status = "  [conflict]"
		case statusUnknown:
			status = "  [unknown]"
		}
		line := prefix + checkbox + body + status
		lines = append(lines, line)
	}

	lines = append(lines, fmt.Sprintf("%d/%d selected", m.selectedCount(), len(m.items)))

	return lines
}

type key int

const (
	keyNone key = iota
	keyUp
	keyDown
	keyToggle
	keyToggleAll
	keyConfirm
	keyCancel
)

func readKey(r *bufio.Reader) (key, error) {
	b, err := r.ReadByte()
	if err != nil {
		return keyNone, err
	}

	switch b {
	case '\r', '\n':
		return keyConfirm, nil
	case ' ':
		return keyToggle, nil
	case 'a':
		return keyToggleAll, nil
	case 'q':
		return keyCancel, nil
	case 0x03:
		return keyCancel, nil
	case 'k':
		return keyUp, nil
	case 'j':
		return keyDown, nil
	case 0x1b:
		if r.Buffered() == 0 {
			return keyCancel, nil
		}
		next, err := r.ReadByte()
		if err != nil {
			return keyCancel, err
		}
		if next != '[' && next != 'O' {
			return keyCancel, nil
		}
		third, err := r.ReadByte()
		if err != nil {
			return keyCancel, err
		}
		switch third {
		case 'A':
			return keyUp, nil
		case 'B':
			return keyDown, nil
		case 'C', 'D':
			return keyNone, nil
		default:
			return keyCancel, nil
		}
	default:
		return keyNone, nil
	}
}

func viewportHeight(out io.Writer, count int) int {
	if !isTerminalWriter(out) {
		return count
	}
	f := out.(*os.File)
	_, height, err := term.GetSize(int(f.Fd()))
	if err != nil || height < 4 {
		return count
	}
	viewport := height - 3
	if viewport < 1 {
		viewport = 1
	}
	if viewport > count {
		viewport = count
	}
	return viewport
}

func fitRenderLines(lines []string, width int) []string {
	if width <= 0 {
		return lines
	}
	fitted := make([]string, len(lines))
	for i, line := range lines {
		fitted[i] = truncateForWidth(line, width-1)
	}
	return fitted
}

func colorizeRenderLines(lines []string, model *selectModel) []string {
	if len(lines) == 0 || model == nil {
		return lines
	}

	styled := append([]string(nil), lines...)
	styled[0] = ansiBold + ansiCyan + styled[0] + ansiReset

	end := model.top + model.height
	if end > len(model.items) {
		end = len(model.items)
	}
	for itemIdx := model.top; itemIdx < end; itemIdx++ {
		lineIdx := 1 + (itemIdx - model.top)
		if lineIdx >= len(styled)-1 {
			break
		}

		prefix := statusColor(model.items[itemIdx].Status)
		if model.checked[itemIdx] {
			prefix = ansiBold + prefix
		}
		styled[lineIdx] = prefix + styled[lineIdx] + ansiReset
	}

	footerColor := ansiDim
	if model.selectedCount() > 0 {
		footerColor = ansiBold + ansiCyan
	}
	styled[len(styled)-1] = footerColor + styled[len(styled)-1] + ansiReset
	return styled
}

func statusColor(status imageStatus) string {
	switch status {
	case statusPushable:
		return ansiYellow
	case statusMirrored:
		return ansiGreen
	case statusConflict:
		return ansiRed
	case statusUnknown:
		return ansiMagenta
	default:
		return ""
	}
}

func writeString(w io.Writer, value string) error {
	_, err := io.WriteString(w, value)
	return err
}

func clearProgressBlockChecked(w io.Writer, lines int) error {
	if lines <= 0 {
		return nil
	}
	if err := writeString(w, "\r\x1b[2K"); err != nil {
		return err
	}
	for i := 1; i < lines; i++ {
		if err := writeString(w, "\x1b[1A\r\x1b[2K"); err != nil {
			return err
		}
	}
	return nil
}

func runSelect(in io.Reader, out io.Writer, items []classifiedImage, registry string) (selected []pushspec.ArchiveSpec, cancelled bool, err error) {
	if len(items) == 0 {
		return nil, false, nil
	}

	height := viewportHeight(out, len(items))
	model := newSelectModel(items, height, registry)

	var state *term.State
	var inputFD int
	hasRawTerminal := false
	if f, ok := in.(*os.File); ok {
		if isTerminalWriter(out) {
			inputFD = int(f.Fd())
			st, makeRawErr := term.MakeRaw(inputFD)
			if makeRawErr != nil {
				return nil, false, fmt.Errorf("enable raw terminal mode: %w", makeRawErr)
			}
			state = st
			hasRawTerminal = true
		}
	}
	if hasRawTerminal {
		defer func() {
			restoreErr := term.Restore(inputFD, state)
			if err == nil && restoreErr != nil {
				err = fmt.Errorf("restore terminal mode: %w", restoreErr)
			}
		}()
	}

	br := bufio.NewReader(in)
	lastLines := 0
	colorOutput := isTerminalWriter(out)

	for {
		lines := fitRenderLines(model.render(), terminalWidth(out))
		if colorOutput {
			lines = colorizeRenderLines(lines, model)
		}
		if lastLines > 0 {
			if err := clearProgressBlockChecked(out, lastLines); err != nil {
				return nil, false, fmt.Errorf("clear interactive output: %w", err)
			}
		}
		for i, line := range lines {
			if i > 0 {
				if err := writeString(out, "\r\n"); err != nil {
					return nil, false, fmt.Errorf("write interactive output: %w", err)
				}
			}
			if err := writeString(out, "\r"+line); err != nil {
				return nil, false, fmt.Errorf("write interactive output: %w", err)
			}
		}
		lastLines = len(lines)

		k, err := readKey(br)
		if err != nil {
			if err == io.EOF {
				if err := writeString(out, "\r\n"); err != nil {
					return nil, false, fmt.Errorf("write interactive output: %w", err)
				}
				return nil, true, nil
			}
			return nil, false, err
		}

		switch k {
		case keyUp:
			model.moveUp()
		case keyDown:
			model.moveDown()
		case keyToggle:
			model.toggle()
		case keyToggleAll:
			model.toggleAll()
		case keyConfirm:
			selected := model.selectedSpecs()
			if err := writeString(out, "\r\n"); err != nil {
				return nil, false, fmt.Errorf("write interactive output: %w", err)
			}
			return selected, false, nil
		case keyCancel:
			if err := writeString(out, "\r\n"); err != nil {
				return nil, false, fmt.Errorf("write interactive output: %w", err)
			}
			return nil, true, nil
		}
	}
}
