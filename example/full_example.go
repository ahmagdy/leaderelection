package main

import (
	"context"
	"fmt"
	"github.com/ahmagdy/leaderelection"
	clientv3 "go.etcd.io/etcd/client/v3"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
)

const (
	_ctxTimeout   = 30 * time.Second
	_dialTimeout  = 5 * time.Second
	_syncInterval = 1 * time.Minute
)

var etcdSeedHostPorts = []string{"localhost:2379", "localhost:22379", "localhost:32379"}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), _ctxTimeout)
	defer cancel()

	osSignal := make(chan os.Signal, 1)
	signal.Notify(osSignal, syscall.SIGINT, syscall.SIGTERM)

	logger, _ := zap.NewProduction()
	instanceName := os.Getenv("INSTANCE_NAME")

	if err := start(ctx, logger, osSignal, instanceName); err != nil {
		logger.Fatal("failed to start the app", zap.Error(err))
	}
}

func start(ctx context.Context, logger *zap.Logger, osSignal chan os.Signal, instanceName string) error {
	etcdClient, err := newETCDClient(etcdSeedHostPorts)
	if err != nil {
		return nil
	}

	defer func() {
		if err := etcdClient.Close(); err != nil {
			logger.Error("received an error while trying to close etcd client connection", zap.Error(err))
		}
	}()

	watcher := &WatcherService{logger: logger}

	leaderElection, err := leaderelection.New(etcdClient, logger, instanceName, watcher)
	if err != nil {
		return err
	}

	if err := leaderElection.Start(ctx); err != nil {
		return fmt.Errorf("failed to start leader election: %w", err)
	}

	defer func() {
		if err := leaderElection.Stop(ctx); err != nil {
			logger.Error("failed to stop leader election", zap.Error(err))
		}
	}()

	<-osSignal
	return nil
}

func newETCDClient(etcdSeedEndpoints []string) (*clientv3.Client, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:        etcdSeedEndpoints,
		DialTimeout:      _dialTimeout,
		AutoSyncInterval: _syncInterval,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}

	return cli, nil
}
