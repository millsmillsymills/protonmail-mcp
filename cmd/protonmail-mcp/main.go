package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "login", "logout", "status":
			fmt.Fprintf(os.Stderr, "subcommand %q not yet implemented\n", os.Args[1])
			os.Exit(2)
		default:
			fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", os.Args[1])
			os.Exit(2)
		}
	}
	fmt.Fprintln(os.Stderr, "MCP server not yet implemented")
	os.Exit(2)
}
