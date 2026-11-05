package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// isTerminal says r is a terminal: stdin when someone types at it. Tests
// replace it.
var isTerminal = func(r io.Reader) bool {
	f, ok := r.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

// errNotConfirmed: the answer to a confirmation wasn't yes.
var errNotConfirmed = errors.New("aborted: nothing written")

// confirm asks question on stderr and reads the answer from stdin, unless
// yes. Without a terminal to ask at, it refuses: a script says --yes.
func confirm(cmd *cobra.Command, yes bool, question string) error {
	if yes {
		return nil
	}
	in := cmd.InOrStdin()
	if !isTerminal(in) {
		return usageError{fmt.Errorf("%s: --yes confirms it when no one is at a terminal to ask", strings.TrimSuffix(question, "?"))}
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "%s [y/N] ", question)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return errNotConfirmed
	}
	if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
		return errNotConfirmed
	}
	return nil
}
