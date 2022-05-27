package leaderelection

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"net/url"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
	"go.etcd.io/etcd/server/v3/etcdserver/api/v3client"
)

const _etcdStartTimeout = 30 * time.Second

// Embedded ETCD instance with tmp directory for serialized key&vals and etcd client.
type Embedded struct {
	tmpDir string
	ETCD   *embed.Etcd
	client *clientv3.Client
	logger *zap.Logger
}

// Start starts embedded ETCD.
// Inspired by https://github.com/ligato/cn-infra/blob/master/db/keyval/etcd/mocks/embeded_etcd.go
func (embd *Embedded) Start(t *testing.T) {
	cfg := embed.NewConfig()
	cfg.Dir = t.TempDir()
	lpurl, _ := url.Parse("http://localhost:0")
	lcurl, _ := url.Parse("http://localhost:0")
	cfg.LPUrls = []url.URL{*lpurl}
	cfg.LCUrls = []url.URL{*lcurl}
	var err error
	embd.ETCD, err = embed.StartEtcd(cfg)
	if err != nil {
		t.Error(err)
		t.FailNow()

	}

	select {
	case <-embd.ETCD.Server.ReadyNotify():
		embd.logger.Debug("Server is ready!")
	case <-time.After(_etcdStartTimeout):
		embd.ETCD.Server.Stop() // trigger a shutdown
		t.Error("Server took too long to start!")
		t.FailNow()
	}
	embd.client = v3client.New(embd.ETCD.Server)
}

// Stop stops the embedded ETCD & cleanups the tmp dir.
func (embd *Embedded) Stop() {
	embd.ETCD.Close()
}

// CleanDs deletes all stored key-value pairs.
func (embd *Embedded) CleanDs() {
	if embd.client != nil {
		resp, err := embd.client.Delete(context.Background(), "", clientv3.WithPrefix())
		if err != nil {
			panic(err)
		}
		fmt.Printf("resp: %+v\n", resp)
	}
}

// Client is a getter for embedded ETCD client.
func (embd *Embedded) Client() *clientv3.Client {
	return embd.client
}
