package cmd

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"tungo/Infrastructure/cmd/handlers"
)

const (
	shutdownCmd           = "exit"
	generateClientConfCmd = "gen"
)

func ListenForCommand(cancelFunc context.CancelFunc, mode string) {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Printf("Type '%s' to turn off the %s\n", shutdownCmd, mode)
	for scanner.Scan() {
		command := strings.TrimSpace(scanner.Text())

		switch {
		case strings.EqualFold(command, shutdownCmd): //handle 'shutdown_cmd' command
			log.Println("Shutting down...")
			cancelFunc()
			return

		case strings.EqualFold(command, generateClientConfCmd): //handle 'generate_new_client_conf_cmd' command
			if err := handlers.GenerateNewClientConf(); err != nil {
				log.Printf("failed to generate new client conf: %s", err)
			}
		default:
			log.Printf("Unknown command: %s", command)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading standard input: %v", err)
	}
}
