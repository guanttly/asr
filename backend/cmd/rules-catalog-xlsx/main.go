package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	apprc "github.com/lgt/asr/internal/application/rulescatalog"
)

func main() {
	sourceDir := flag.String("source", "../docs/rules", "rules catalog source directory")
	scope := flag.String("scope", "", "department directory or markdown file relative to source")
	outPath := flag.String("out", "", "xlsx output path")
	flag.Parse()

	if *outPath == "" {
		_, _ = fmt.Fprintln(os.Stderr, "missing required -out")
		os.Exit(2)
	}

	svc := apprc.NewService(*sourceDir)
	var buf bytes.Buffer
	count, err := svc.GenerateXLSX(&buf, *scope)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "generate xlsx: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "create output dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*outPath, buf.Bytes(), 0o644); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "write xlsx: %v\n", err)
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(os.Stderr, "wrote %s (%d rules)\n", *outPath, count)
}
