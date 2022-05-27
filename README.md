# etcd-leaderelection
A go leader election module that can be used to track leader changes.


### Install
```bash
go get -u github.com/ahmagdy/leaderelection 
```

### Use case:
This package is intended to be used when multiple instances are running of service, and only a single instance can be the leader, or a single instance can do a specific job.

### Example:
It's mostly intended for services built on top of a framework that supports app lifecycle. e.g. [Uber fx](https://github.com/uber-go/fx).
```go
leaderElection := leaderelection.New(etcdClient, zapLogger, instanceName, watcher)

fx.Invoke(func(lc fx.Lifecycle) {
    lc.Append(fx.Hook{
        OnStart: func(context.Context) error { return leaderElection.Start(ctx) },
        OnStop: func(context.Context) error { return leaderElection.Stop(ctx) },
    })
})

```

##### Without lifecycle support:
```go
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

	defer func() {
		if err := leaderElection.Stop(ctx); err != nil {
			logger.Error("failed to stop leader election", zap.Error(err))
		}
	}()
	
}

```


#### Watcher interface to act on leadership changes
I tried to keep the interface minimal without fancy options.

Feel free to contribute or open a ticket if you feel like it's missing an event or can be extended.

```go
var _ leaderelection.EventsWatcher = (*WatcherService)(nil)

type WatcherService struct {
	logger *zap.Logger
}

func (w *WatcherService) OnGainedLeadership() {
	w.logger.Info("I am the new leader")
}

func (w *WatcherService) OnLostLeadership() {
	w.logger.Info("I am no longer the leader")
}

```

Please check the `/example` package for more go examples.

## License:
[Apache License 2.0](https://github.com/ahmagdy/leaderelection/blob/main/LICENSE)
