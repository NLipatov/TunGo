package interactive_commands

import (
	"bufio"
	"log"
	"os"
	"strings"
	"tungo/presentation/interactive_commands/handlers"
)

const (
	generateClientConfCmd = "gen"
)

func ListenForCommand() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		command := strings.TrimSpace(scanner.Text())

		switch {
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
