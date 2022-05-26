package main

import (
	"context"
	mock_main "github.com/ahmagdy/etcd-leaderelection/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/zap/zaptest"
	"testing"
)

func TestMain(m *testing.M) {
	opts := []goleak.Option{
		goleak.IgnoreTopFunction("go.etcd.io/etcd/raft/v3.(*node).run"),
		goleak.IgnoreTopFunction("go.etcd.io/etcd/server/v3/mvcc/backend.(*backend).run"),
		goleak.IgnoreTopFunction("go.etcd.io/etcd/server/v3/wal.(*filePipeline).run"),
		goleak.IgnoreTopFunction("go.etcd.io/etcd/server/v3/lease.(*lessor).runLoop"),
		goleak.IgnoreTopFunction("go.etcd.io/etcd/client/pkg/v3/fileutil.purgeFile.func1"),
		goleak.IgnoreTopFunction("go.etcd.io/etcd/pkg/v3/schedule.(*fifo).run"),
		goleak.IgnoreTopFunction("go.etcd.io/etcd/server/v3/mvcc.(*watchableStore).syncWatchersLoop"),
		goleak.IgnoreTopFunction("go.etcd.io/etcd/client/v3.(*watchGrpcStream).serveSubstream"),
		goleak.IgnoreTopFunction("go.etcd.io/etcd/server/v3/proxy/grpcproxy/adapter.(*chanStream).RecvMsg"),
		goleak.IgnoreTopFunction("go.etcd.io/etcd/client/v3.(*lessor).sendKeepAliveLoop"),
		goleak.IgnoreTopFunction("go.etcd.io/etcd/client/v3.(*lessor).deadlineLoop"),
		goleak.IgnoreTopFunction("go.etcd.io/etcd/client/v3.(*lessor).keepAliveCtxCloser"),
		goleak.IgnoreTopFunction("go.etcd.io/etcd/client/v3/concurrency.NewSession.func1"),
		goleak.IgnoreTopFunction("go.etcd.io/etcd/server/v3/etcdserver/api/v3rpc.(*serverWatchStream).sendLoop"),
		goleak.IgnoreTopFunction("go.etcd.io/etcd/server/v3/etcdserver/api/v3rpc.(*watchServer).Watch"),
		goleak.IgnoreTopFunction("go.etcd.io/etcd/client/v3.(*watchGrpcStream).run"),
		goleak.IgnoreTopFunction("go.etcd.io/etcd/client/v3/concurrency.(*Election).observe"),
		goleak.IgnoreTopFunction("go.etcd.io/etcd/server/v3/etcdserver/api/v3rpc.(*LeaseServer).LeaseKeepAlive"),
		goleak.IgnoreTopFunction("go.etcd.io/etcd/client/v3.(*lessor).recvKeepAliveLoop"),
	}
	goleak.VerifyTestMain(m, opts...)
}

func Test_Foo(t *testing.T) {
	const _fooInstance = "foo"
	const _barInstance = "bar"

	t.Run("onGainedLeadership is called when there is no participants", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		ctrl := gomock.NewController(t)
		logger := zaptest.NewLogger(t)
		e := &Embedded{logger: logger}

		leadershipEventsWatcher := mock_main.NewMockLeadershipEventsWatcher(ctrl)
		leadershipEventsWatcher.EXPECT().OnGainedLeadership()

		e.Start(t)
		t.Cleanup(func() {
			e.CleanDs()
			e.Stop()
			cancel()
		})

		leaderElection, err := NewLeaderElection(e.Client(), logger, _fooInstance, leadershipEventsWatcher)
		require.NoError(t, err)
		require.NoError(t, leaderElection.Start(ctx))
	})

	t.Run("when a new participant joins, and the leader is healthy, no new election occurs", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		ctrl := gomock.NewController(t)
		logger := zaptest.NewLogger(t)
		e := &Embedded{logger: logger}
		fooLeadershipEventsWatcher := mock_main.NewMockLeadershipEventsWatcher(ctrl)
		barLeadershipEventsWatcher := mock_main.NewMockLeadershipEventsWatcher(ctrl)
		e.Start(t)
		t.Cleanup(func() {
			e.CleanDs()
			e.Stop()
			cancel()
		})

		fooLeadershipEventsWatcher.EXPECT().OnGainedLeadership()

		fooSvcLeaderElection, err := NewLeaderElection(e.Client(), logger, _fooInstance, fooLeadershipEventsWatcher)
		require.NoError(t, err)
		require.NoError(t, fooSvcLeaderElection.Start(ctx))

		barSvcLeaderElection, err := NewLeaderElection(e.Client(), logger, _barInstance, barLeadershipEventsWatcher)
		require.NoError(t, err)
		require.NoError(t, barSvcLeaderElection.Start(ctx))
	})

	t.Run("when the leader stops, a new leader from the participants gets elected", func(t *testing.T) {
		// Note, there is a potential raise in this test
		ctx, cancel := context.WithCancel(context.Background())
		ctrl := gomock.NewController(t)
		logger := zaptest.NewLogger(t)
		wait := make(chan struct{}, 2)
		e := &Embedded{logger: logger}
		fooLeadershipEventsWatcher := mock_main.NewMockLeadershipEventsWatcher(ctrl)
		barLeadershipEventsWatcher := mock_main.NewMockLeadershipEventsWatcher(ctrl)
		e.Start(t)
		t.Cleanup(func() {
			e.CleanDs()
			e.Stop()
			cancel()
		})

		fooLeadershipEventsWatcher.EXPECT().OnGainedLeadership()
		fooLeadershipEventsWatcher.EXPECT().OnLostLeadership().Do(func() { wait <- struct{}{} })
		barLeadershipEventsWatcher.EXPECT().OnGainedLeadership().Do(func() { wait <- struct{}{} })

		fooSvcLeaderElection, err := NewLeaderElection(e.Client(), logger, _fooInstance, fooLeadershipEventsWatcher)
		require.NoError(t, err)
		require.NoError(t, fooSvcLeaderElection.Start(ctx))

		barSvcLeaderElection, err := NewLeaderElection(e.Client(), logger, _barInstance, barLeadershipEventsWatcher)
		require.NoError(t, err)
		require.NoError(t, barSvcLeaderElection.Start(ctx))

		require.NoError(t, fooSvcLeaderElection.Stop(ctx))
		// block both OnLostLeadership and second OnGainedLeadership
		// so the test doesn't exist before they are satisfied
		<-wait
		<-wait
	})

	t.Run("when the leader resigns, a new leader from the participants gets elected", func(t *testing.T) {
		// setup
		ctx, cancel := context.WithCancel(context.Background())
		ctrl := gomock.NewController(t)
		logger := zaptest.NewLogger(t)
		wait := make(chan struct{}, 2)
		e := &Embedded{logger: logger}
		fooLeadershipEventsWatcher := mock_main.NewMockLeadershipEventsWatcher(ctrl)
		barLeadershipEventsWatcher := mock_main.NewMockLeadershipEventsWatcher(ctrl)

		e.Start(t)
		t.Cleanup(func() {
			e.CleanDs()
			e.Stop()
			cancel()
		})

		// expect
		fooLeadershipEventsWatcher.EXPECT().OnGainedLeadership()
		fooLeadershipEventsWatcher.EXPECT().OnLostLeadership().Do(func() { wait <- struct{}{} })
		barLeadershipEventsWatcher.EXPECT().OnGainedLeadership().Do(func() { wait <- struct{}{} })

		// assert
		fooSvcLeaderElection, err := NewLeaderElection(e.Client(), logger, _fooInstance, fooLeadershipEventsWatcher)
		require.NoError(t, err)
		require.NoError(t, fooSvcLeaderElection.Start(ctx))

		barSvcLeaderElection, err := NewLeaderElection(e.Client(), logger, _barInstance, barLeadershipEventsWatcher)
		require.NoError(t, err)
		require.NoError(t, barSvcLeaderElection.Start(ctx))

		require.NoError(t, fooSvcLeaderElection.Resign(ctx))
		// block both OnLostLeadership and second OnGainedLeadership
		// so the test doesn't exist before they are satisfied
		<-wait
		<-wait
	})

}
