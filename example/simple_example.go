package main

import (
	"context"
	"fmt"
	"github.com/ahmagdy/leaderelection"
	"go.uber.org/zap"
	"os"
)

func simpleMain() {
	ctx, cancel := context.WithTimeout(context.Background(), _ctxTimeout)
	defer cancel()
	logger, _ := zap.NewProduction()
	instanceName := os.Getenv("INSTANCE_NAME")

	if err := run(ctx, logger, instanceName); err != nil {
		logger.Fatal("failed to start the app", zap.Error(err))
	}

}

func run(ctx context.Context, logger *zap.Logger, instanceName string) error {
	etcdClient, err := newETCDClient(etcdSeedHostPorts)
	if err != nil {
		return nil
	}

	watcher := &WatcherService{logger: logger}

	leaderElection, err := leaderelection.New(etcdClient, logger, instanceName, watcher)
	if err != nil {
		return err
	}

	if err := leaderElection.Start(ctx); err != nil {
		return fmt.Errorf("failed to start leader election: %w", err)
	}

	return nil
}
