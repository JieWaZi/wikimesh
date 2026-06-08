package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/JieWaZi/wikimesh/internal/cli"
)

func main() {
	os.Exit(run(os.Stderr))
}

func run(stderr io.Writer) int {
	if err := cli.NewRootCmd().Execute(); err != nil {
		return reportCLIError(err, stderr)
	}
	return 0
}

func reportCLIError(err error, stderr io.Writer) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	_, _ = fmt.Fprintln(stderr, err)
	return 1
}
