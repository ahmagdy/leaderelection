package main

import (
	"context"
	"errors"
	"fmt"
	"sync"

	clientv3 "go.etcd.io/etcd/client/v3"

	"go.etcd.io/etcd/client/v3/concurrency"
	"go.uber.org/zap"
)

const (
	_electionPrefix = "/leader-election"
)

var _ Election = (*concurrency.Election)(nil)

type Election interface {
	Campaign(ctx context.Context, val string) error
	Resign(ctx context.Context) (err error)
	Proclaim(ctx context.Context, val string) error
	Leader(ctx context.Context) (*clientv3.GetResponse, error)
	Observe(ctx context.Context) <-chan clientv3.GetResponse
}

type LeadershipEventsWatcher interface {
	OnGainedLeadership()
	OnLostLeadership()
}

type LeaderElection struct {
	etcdClient              Client
	election                Election
	logger                  *zap.Logger
	leadershipEventsWatcher LeadershipEventsWatcher
	instanceName            string
	watcherExitChan         chan struct{}

	mux           sync.Mutex
	currentLeader string // protected by mux
}

func NewLeaderElection(etcdClient Client, logger *zap.Logger, instanceName string, leadershipEventsWatcher LeadershipEventsWatcher) *LeaderElection {

	return &LeaderElection{
		etcdClient:              etcdClient,
		logger:                  logger,
		instanceName:            instanceName,
		watcherExitChan:         make(chan struct{}),
		leadershipEventsWatcher: leadershipEventsWatcher,
	}
}

// Start starts the leader election process.
// If there is already a leader, it will become a follower and participate in the election process.
// If there is no leader, it will start the election process.
func (l *LeaderElection) Start(ctx context.Context) error {
	l.election = concurrency.NewElection(l.etcdClient.Session(), _electionPrefix)

	go l.observeChanges(ctx)

	leader, err := l.election.Leader(ctx)
	// There is a leader
	if err == nil {
		go l.campaign(ctx)

		l.mux.Lock()
		defer l.mux.Unlock()
		l.currentLeader = string(leader.Kvs[0].Value)
		l.logger.Info("this instance is not the leader", zap.String("leaderName", l.currentLeader))
		return nil
	}

	if !errors.Is(err, concurrency.ErrElectionNoLeader) {
		return fmt.Errorf("failed to get leader: %w", err)
	}

	l.logger.Info("there is no leader at the moment. trying to be elected")
	// become a leader
	l.campaign(ctx)
	return nil
}

// Stop stops the leader election process.
func (l *LeaderElection) Stop(ctx context.Context) error {
	if l.election == nil {
		return nil
	}

	err := l.election.Resign(ctx)

	<-l.watcherExitChan
	l.election = nil
	return err
}

// If the instance is not the leader it will participate,
// so the instance can potentially become the leader once the existing leader resigns
func (l *LeaderElection) campaign(ctx context.Context) {
	if err := l.election.Campaign(ctx, l.instanceName); err != nil {
		l.logger.Error("failed to campaign", zap.Error(err))
		return
	}

	l.mux.Lock()
	defer l.mux.Unlock()
	l.currentLeader = l.instanceName

	l.onGainedLeadership()

	l.logger.Info("look at me, i'm the boss now")
}

func (l *LeaderElection) observeChanges(ctx context.Context) {
	// observe reports when the leader is changed
	eventsChan := l.election.Observe(ctx)
	for event := range eventsChan {
		l.processEvent(event)
	}

	close(l.watcherExitChan)
}

func (l *LeaderElection) processEvent(event clientv3.GetResponse) {
	key := string(event.Kvs[0].Key)
	leader := string(event.Kvs[0].Value)

	l.logger.Info("received a new leadership event", zap.String("key", key), zap.String("leader", leader))

	// skip reporting if the leader is the current instance as it will be reported by onGainedLeadership
	if leader == l.instanceName {
		return
	}

	l.mux.Lock()
	defer l.mux.Unlock()
	if l.currentLeader == l.instanceName {
		l.currentLeader = leader
		l.logger.Info("lost leadership", zap.String("leader", leader))
		l.onLostLeadership()
	}

}

func (l *LeaderElection) onGainedLeadership() {
	if l.leadershipEventsWatcher != nil {
		l.leadershipEventsWatcher.OnGainedLeadership()
	}
}

func (l *LeaderElection) onLostLeadership() {
	if l.leadershipEventsWatcher != nil {
		l.leadershipEventsWatcher.OnLostLeadership()
	}
}
