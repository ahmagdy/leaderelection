package main

import "go.uber.org/zap"

var _ LeadershipEventsWatcher = (*WatcherService)(nil)

type WatcherService struct {
	logger *zap.Logger
}

func (w *WatcherService) OnGainedLeadership() {
	w.logger.Info("I am the new leader")
}

func (w *WatcherService) OnLostLeadership() {
	w.logger.Info("I am no longer the leader")
}
