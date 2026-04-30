package registry

import (
	"fmt"
	"io"
	"os"
	"sync"
)

// deprecationWriter is the destination for legacy-layout deprecation warnings.
// Defaults to os.Stderr. Tests override via SetDeprecationWriter.
var (
	deprecationMu     sync.Mutex
	deprecationWriter io.Writer = os.Stderr
	// QuietDeprecations suppresses all legacy-layout warnings. Set this from
	// the CLI when --quiet is in effect (e.g. session-start hooks) so the
	// warning doesn't become noise on every invocation.
	QuietDeprecations bool
	warnedRegistries  = map[string]struct{}{}
)

// SetDeprecationWriter overrides the warning destination. Returns the previous
// writer so tests can restore it. Pass nil to silence completely.
func SetDeprecationWriter(w io.Writer) io.Writer {
	deprecationMu.Lock()
	defer deprecationMu.Unlock()
	prev := deprecationWriter
	deprecationWriter = w
	return prev
}

// ResetDeprecationState clears the per-registry de-duplication map so each
// call to a v1 registry triggers a warning again. Intended for tests.
func ResetDeprecationState() {
	deprecationMu.Lock()
	defer deprecationMu.Unlock()
	warnedRegistries = map[string]struct{}{}
}

// emitLegacyLayoutWarning prints a one-line stderr warning the first time the
// process loads a v1 (nested) registry. De-duplicated by owner/repo so a
// command that hits the same registry several times doesn't spam.
func emitLegacyLayoutWarning(owner, repo string) {
	if QuietDeprecations {
		return
	}
	deprecationMu.Lock()
	defer deprecationMu.Unlock()
	if deprecationWriter == nil {
		return
	}
	key := owner + "/" + repo
	if _, seen := warnedRegistries[key]; seen {
		return
	}
	warnedRegistries[key] = struct{}{}
	fmt.Fprintf(deprecationWriter,
		"warning: registry %s uses the legacy .amaru_registry/ layout — run `amaru repo migrate` before this layout is removed in a future release.\n",
		key,
	)
}
