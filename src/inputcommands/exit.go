package inputcommands

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
)

func ListenForExitCommand(cancelFunc context.CancelFunc, shutdownCommand string) {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Printf("Type '%s' to turn off the client\n", shutdownCommand)
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if strings.EqualFold(text, shutdownCommand) {
			log.Println("Exit command received. Shutting down...")
			cancelFunc()
			return
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("Error reading standard input: %v", err)
	}
}
