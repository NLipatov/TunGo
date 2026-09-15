package main

import "context"

func runTunnel(ctx context.Context, server *server, protocol, directory, artifacts string) error {
	checksum, err := server.checkTarget(ctx)
	if err != nil {
		return err
	}
	config, err := server.startServer(ctx, protocol)
	if err != nil {
		return err
	}
	if err := runRemoteClient(ctx, directory, artifacts, clientInput{Config: config, Checksum: checksum}); err != nil {
		return err
	}
	return server.stopServer(ctx)
}
