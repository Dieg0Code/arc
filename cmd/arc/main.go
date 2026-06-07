// Command arc versiona el contexto de los agentes como git versiona el código.
package main

import (
	"fmt"
	"os"

	"github.com/Dieg0Code/arc/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
