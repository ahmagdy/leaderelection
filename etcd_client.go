package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"go.uber.org/zap"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"go.uber.org/multierr"
)

const (
	_dialTimeout  = 5 * time.Second
	_syncInterval = 1 * time.Minute

	_lockPrefix = "/v1/lock/"
)

var _ Client = (*client)(nil)

type Client interface {
	clientv3.KV
	clientv3.Watcher
	io.Closer
	WatchAndPrintEvents(ctx context.Context, key string, opts ...clientv3.OpOption) error
	Lock(ctx context.Context, lockName string) (func(ctx context.Context) error, error)
	Session() *concurrency.Session
}

type client struct {
	logger      *zap.Logger
	etcdClient  *clientv3.Client
	etcdSession *concurrency.Session
	kvClient    clientv3.KV
}

func (c *client) RequestProgress(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}

func New(logger *zap.Logger) (Client, error) {
	etcdClient, err := newETCDClient()
	if err != nil {
		return nil, err
	}

	kvClient := clientv3.NewKV(etcdClient)

	etcdSession, err := concurrency.NewSession(etcdClient, concurrency.WithTTL(1 /* second */))
	if err != nil {
		return nil, err
	}

	return &client{
		logger:      logger,
		etcdClient:  etcdClient,
		kvClient:    kvClient,
		etcdSession: etcdSession,
	}, nil
}

func newETCDClient() (*clientv3.Client, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:        []string{"localhost:2379", "localhost:22379", "localhost:32379"},
		DialTimeout:      _dialTimeout,
		AutoSyncInterval: _syncInterval,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}

	return cli, nil
}

func (c *client) Session() *concurrency.Session {
	return c.etcdSession
}

func (c *client) Close() error {
	err := c.etcdSession.Close()
	etcdErr := c.etcdClient.Close()
	return multierr.Append(err, etcdErr)
}

func (c *client) Watch(ctx context.Context, key string, opts ...clientv3.OpOption) clientv3.WatchChan {
	return c.etcdClient.Watch(ctx, key, opts...)
}

func (c *client) WatchAndPrintEvents(ctx context.Context, key string, opts ...clientv3.OpOption) error {
	watcher := c.etcdClient.Watch(ctx, key, opts...)
	for {
		select {
		case <-ctx.Done():
			return nil
		case wresp, ok := <-watcher:
			if !ok {
				return errors.New("the watcher connection was closed")
			}
			for _, ev := range wresp.Events {
				fmt.Printf("Got a new event: %s %q : %q\n", ev.Type, ev.Kv.Key, ev.Kv.Value)
			}
		}
	}
}

func (c *client) Put(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
	return c.kvClient.Put(ctx, key, val, opts...)
}

func (c *client) Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	return c.kvClient.Get(ctx, key, opts...)
}

func (c *client) Delete(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.DeleteResponse, error) {
	return c.kvClient.Delete(ctx, key, opts...)
}

func (c *client) Compact(ctx context.Context, rev int64, opts ...clientv3.CompactOption) (*clientv3.CompactResponse, error) {
	return c.kvClient.Compact(ctx, rev, opts...)
}

func (c *client) Do(ctx context.Context, op clientv3.Op) (clientv3.OpResponse, error) {
	return c.kvClient.Do(ctx, op)
}

func (c *client) Txn(ctx context.Context) clientv3.Txn {
	return c.kvClient.Txn(ctx)
}

func (c *client) Lock(ctx context.Context, lockName string) (func(ctx context.Context) error, error) {
	locker := concurrency.NewMutex(c.etcdSession, _lockPrefix+lockName)

	// Locker will panic if unable to take a lock. BAD
	//err := locker.Lock(ctx)

	err := locker.TryLock(ctx)
	if err == nil {
		c.logger.Info("fine", zap.Int64("leaseID", int64(c.etcdSession.Lease())))
		return locker.Unlock, nil
	}

	c.logger.Info("not fine")
	if err == concurrency.ErrLocked {
		c.logger.Info("locked")
	} else {
		c.logger.Info(err.Error())
	}

	return nil, err

}
