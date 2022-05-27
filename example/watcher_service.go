package main

import (
	"github.com/ahmagdy/leaderelection"
	"go.uber.org/zap"
)

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
