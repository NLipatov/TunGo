package main

import (
	"etha-tunnel/net"
	"fmt"
	"log"
)

const (
	ifName = "ethatun0"
)

func main() {
	name, err := net.UpNewTun(ifName)
	if err != nil {
		log.Fatalf("failed to create interface %v: %v", ifName, err)
	}
	defer func() {
		err = net.DeleteInterface(ifName)
		if err != nil {
			log.Fatalf("failed to delete interface %v: %v", ifName, err)
		}
		fmt.Printf("%s interface deleted\n", ifName)
	}()

	fmt.Printf("Created: %v", name)
}
