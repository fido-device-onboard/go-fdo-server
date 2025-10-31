//go:build !windows

package devlog

import (
	"io"
	"os"

	"golang.org/x/term"
)

// IsColorTerminal checks if the given writer is a terminal with ANSI color support.
// It respects [NO_COLOR], [FORCE_COLOR] and TERM=dumb environment variables.
//
// [NO_COLOR]: https://no-color.org/
// [FORCE_COLOR]: https://force-color.org/
func IsColorTerminal(output io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("FORCE_COLOR") != "" {
		return true
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}

	if output == nil {
		return false
	}

	file, isFile := output.(*os.File)
	if !isFile {
		return false
	}

	if !term.IsTerminal(int(file.Fd())) {
		return false
	}

	return true
}
