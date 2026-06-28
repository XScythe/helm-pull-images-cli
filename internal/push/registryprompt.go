package push

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"helm-deep-pack/internal/validation"
)

// errRegistryRequired signals that no registry was provided and none could be
// obtained: either the prompt was cancelled (empty line, EOF) or no terminal
// was available to prompt on.
var errRegistryRequired = errors.New("registry argument is required")

// promptForRegistry obtains a registry interactively when one was not supplied.
// It mirrors selectImagesToPush: interaction requires a terminal on both input
// and output; otherwise it fails fast (rather than blocking on a read) so
// non-interactive callers get a clear, deterministic error.
func promptForRegistry(in io.Reader, out io.Writer) (string, error) {
	if in == nil || !isTerminalReader(in) || out == nil || !isTerminalWriter(out) {
		return "", errRegistryRequired
	}
	return readRegistryLoop(in, out)
}

// readRegistryLoop prompts on out and reads lines from in until a valid registry
// is entered, re-prompting on invalid input. Validation reuses the same
// validator as the positional argument so typed and supplied registries share
// identical semantics. An empty submission or end of input cancels and returns
// errRegistryRequired.
func readRegistryLoop(in io.Reader, out io.Writer) (string, error) {
	reader := bufio.NewReader(in)
	for {
		if _, err := fmt.Fprint(out, "Target registry: "); err != nil {
			return "", fmt.Errorf("write registry prompt: %w", err)
		}

		line, readErr := reader.ReadString('\n')
		registry := strings.TrimSpace(line)

		if registry != "" {
			if valErr := validation.ValidateImageRegistryWithPath("registry argument", registry); valErr == nil {
				return registry, nil
			} else if _, err := fmt.Fprintf(out, "invalid registry %q: %v\n", registry, valErr); err != nil {
				return "", fmt.Errorf("write registry prompt: %w", err)
			}
		}

		// No more input means we cannot re-prompt; an empty submission is an
		// explicit cancel. Either way, the registry stays unresolved.
		if readErr != nil || registry == "" {
			return "", errRegistryRequired
		}
	}
}
