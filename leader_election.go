package leaderelection

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

type EventsWatcher interface {
	OnGainedLeadership()
	OnLostLeadership()
}

type Service struct {
	etcdSession             *concurrency.Session
	election                *concurrency.Election
	logger                  *zap.Logger
	leadershipEventsWatcher EventsWatcher
	instanceName            string
	watcherExitChan         chan struct{}

	mux           sync.Mutex
	currentLeader string // protected by mux
}

func New(etcdClient *clientv3.Client, logger *zap.Logger, instanceName string, leadershipEventsWatcher EventsWatcher) (*Service, error) {
	etcdSession, err := concurrency.NewSession(etcdClient, concurrency.WithTTL(1 /* second */))
	if err != nil {
		return nil, err
	}

	return &Service{
		etcdSession:             etcdSession,
		logger:                  logger,
		instanceName:            instanceName,
		watcherExitChan:         make(chan struct{}),
		leadershipEventsWatcher: leadershipEventsWatcher,
	}, nil
}

// Start starts the leader election process.
// If there is already a leader, it will become a follower and participate in the election process.
// If there is no leader, it will start the election process.
func (l *Service) Start(ctx context.Context) error {
	l.election = concurrency.NewElection(l.etcdSession, _electionPrefix)

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
func (l *Service) Stop(ctx context.Context) error {
	if l.election == nil {
		return nil
	}

	err := l.Resign(ctx)

	//<-l.watcherExitChan
	l.election = nil
	return err
}

// Resign Lets the leader abdicate its leadership
func (l *Service) Resign(ctx context.Context) error {
	if !l.isCurrentInstanceLeader() {
		return nil
	}

	return l.election.Resign(ctx)
}

// If the instance is not the leader it will participate,
// so the instance can potentially become the leader once the existing leader resigns
func (l *Service) campaign(ctx context.Context) {
	if err := l.election.Campaign(ctx, l.instanceName); err != nil {
		l.logger.Error("failed to campaign", zap.Error(err))
		return
	}

	l.mux.Lock()
	defer l.mux.Unlock()
	l.currentLeader = l.instanceName

	l.leadershipEventsWatcher.OnGainedLeadership()

	l.logger.Info("look at me, i'm the boss now")
}

func (l *Service) observeChanges(ctx context.Context) {
	// observe reports when the leader is changed
	eventsChan := l.election.Observe(ctx)
	for event := range eventsChan {
		l.processEvent(event)
	}

	close(l.watcherExitChan)
}

func (l *Service) processEvent(event clientv3.GetResponse) {
	if len(event.Kvs) == 0 {
		l.logger.Warn("received election event with no keys", zap.Any("headers", event.Header))
		return
	}

	key := string(event.Kvs[0].Key)
	leader := string(event.Kvs[0].Value)

	l.logger.Info("received a new leadership event", zap.String("key", key), zap.String("leader", leader))

	// skip reporting if the leader is the current instance as it will be reported by onGainedLeadership
	if leader == l.instanceName {
		return
	}

	l.mux.Lock()
	defer l.mux.Unlock()
	if l.isCurrentInstanceLeader() {
		l.currentLeader = leader
		l.logger.Info("lost leadership", zap.String("leader", leader))
		l.leadershipEventsWatcher.OnLostLeadership()
	}

}

func (l *Service) isCurrentInstanceLeader() bool {
	return l.currentLeader == l.instanceName
}
