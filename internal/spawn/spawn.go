// Package spawn opens new voxRobota terminal windows.
package spawn

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// NewWindow launches a new terminal running the current binary with working
// directory dir. template tokens {dir} and {bin} are substituted.
func NewWindow(template []string, dir string) error {
	if len(template) == 0 {
		return fmt.Errorf("no terminal template configured")
	}
	bin, err := os.Executable()
	if err != nil || bin == "" {
		bin = os.Args[0]
	}
	args := make([]string, 0, len(template))
	for _, t := range template {
		t = strings.ReplaceAll(t, "{dir}", dir)
		t = strings.ReplaceAll(t, "{bin}", bin)
		args = append(args, t)
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	return cmd.Start()
}
