package inputcommands

import (
	"bufio"
	"context"
	"encoding/json"
	"etha-tunnel/connectionconfgeneration"
	"fmt"
	"log"
	"os"
	"strings"
)

const (
	shutdown           = "exit"
	generateClientConf = "gen"
)

func ListenForCommand(cancelFunc context.CancelFunc) {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Printf("Type '%s' to turn off the client\n", shutdown)
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if strings.EqualFold(text, shutdown) {
			log.Println("Exit command received. Shutting down...")
			cancelFunc()
			return
		} else if strings.EqualFold(text, generateClientConf) {
			err := printNewClientConf()
			if err != nil {
				log.Printf("failed to generate new client conf: %s", err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("Error reading standard input: %v", err)
	}
}

func printNewClientConf() error {
	newConf, err := connectionconfgeneration.Generate()
	if err != nil {
		log.Fatalf("failed to generate client conf: %s\n", err)
	}

	marshalled, err := json.MarshalIndent(newConf, "", "  ")
	if err != nil {
		log.Fatalf("failed to marshalize client conf: %s\n", err)
	}

	fmt.Println(string(marshalled))
	return nil
}
