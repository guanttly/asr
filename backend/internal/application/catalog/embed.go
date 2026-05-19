package catalog

import "embed"

// termsFS embeds a read-only snapshot of the radiology terminology catalog.
//
// The canonical markdown sources live in repo `docs/terms/`. The files under
// `./terms/` here are a build-time copy that the backend serves as static
// content. Run `make sync-term-catalog` from the repo root to refresh this
// snapshot — never edit the files under this directory directly.
//
//go:embed terms
var termsFS embed.FS
