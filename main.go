// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of tastecheck

package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/chez-shanpu/tastecheck/cmd"
)

// main exits with 1 both when the code is below a threshold and when the
// check could not be completed. The output tells the two apart: a violation
// is reported on stdout, while a tool failure is reported as "Error: ..." on
// stderr.
func main() {
	err := cmd.Execute()
	if err == nil {
		return
	}
	// On a violation the report has already been written.
	if !errors.Is(err, cmd.ErrTasteViolation) {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
	os.Exit(1)
}
