package rulescatalog

import "embed"

// rulesFS embeds a read-only snapshot of the radiology rules catalog.
//
// The canonical markdown sources live in repo `docs/rules/`. The files under
// `./rules/` here are a build-time copy that the backend serves as static
// content. Run `make sync-rules-catalog` from the repo root to refresh this
// snapshot — never edit the files under this directory directly.
//
//go:embed rules
var rulesFS embed.FS
