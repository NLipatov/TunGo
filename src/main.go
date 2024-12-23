package main

import (
	"fmt"
	"os"
	presentation2 "tungo/Infrastructure/presentation"
)

const (
	PackageName = "tungo"
	ServerMode  = "s"
	ClientMode  = "c"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	mode := os.Args[1]

	switch mode {
	case ServerMode:
		fmt.Println("🚀 Starting server...")
		presentation2.StartServer()
	case ClientMode:
		fmt.Println("🛡️ Starting client...")
		presentation2.StartClient()
	default:
		fmt.Printf("Unknown mode: %s\n", mode)
		printUsage()
	}
}

func printUsage() {
	fmt.Printf(`Usage: %s <mode>
args:
  "s" - server
  "c" - client
`, PackageName)
}
