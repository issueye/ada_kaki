// AdaKaki is a desktop console: multiple shells and SSH sessions in one
// window, popped out to more, driven by AI agents over MCP.
//
//	ada_kaki            the console (GUI)
//	ada_kaki mcp        the MCP server over stdio, for AI agents
//	ada_kaki version    print the version
package main

import (
	"fmt"
	"os"

	"adakaki/ui"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "mcp":
			runMCPStdio(os.Args[2:])
			return
		case "version", "-v", "--version":
			fmt.Println("adakaki v0.1.0")
			return
		default:
			fmt.Fprintf(os.Stderr, "adakaki: unknown command %q (want mcp or version)\n", os.Args[1])
			os.Exit(2)
		}
	}
	ui.RunGUI()
}
