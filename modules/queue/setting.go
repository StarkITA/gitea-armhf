// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package queue

import (
	"fmt"
	"strings"

	"code.gitea.io/gitea/modules/json"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/setting"
)

func validType(t string) (Type, error) {
	if len(t) == 0 {
		return PersistableChannelQueueType, nil
	}
	for _, typ := range RegisteredTypes() {
		if t == string(typ) {
			return typ, nil
		}
	}
	return PersistableChannelQueueType, fmt.Errorf("unknown queue type: %s defaulting to %s", t, string(PersistableChannelQueueType))
}

func getQueueSettings(name string) (setting.QueueSettings, []byte) {
	q := setting.GetQueueSettings(name)
	cfg, err := json.Marshal(q)
	if err != nil {
		log.Error("Unable to marshall generic options: %v Error: %v", q, err)
		log.Error("Unable to create queue for %s", name, err)
		return q, []byte{}
	}
	return q, cfg
}

// CreateQueue for name with provided handler and exemplar
func CreateQueue(name string, handle HandlerFunc, exemplar interface{}) Queue {
	q, cfg := getQueueSettings(name)
	if len(cfg) == 0 {
		return nil
	}

	typ, err := validType(q.Type)
	if err != nil {
		log.Error("Invalid type %s provided for queue named %s defaulting to %s", q.Type, name, string(typ))
	}

	returnable, err := NewQueue(typ, handle, cfg, exemplar)
	if q.WrapIfNecessary && err != nil {
		log.Warn("Unable to create queue for %s: %v", name, err)
		log.Warn("Attempting to create wrapped queue")
		returnable, err = NewQueue(WrappedQueueType, handle, WrappedQueueConfiguration{
			Underlying:  typ,
			Timeout:     q.Timeout,
			MaxAttempts: q.MaxAttempts,
			Config:      cfg,
			QueueLength: q.QueueLength,
			Name:        name,
		}, exemplar)
	}
	if err != nil {
		log.Error("Unable to create queue for %s: %v", name, err)
		return nil
	}

	// Sanity check configuration
	if q.Workers == 0 && (q.BoostTimeout == 0 || q.BoostWorkers == 0 || q.MaxWorkers == 0) {
		log.Warn("Queue: %s is configured to be non-scaling and have no workers\n - this configuration is likely incorrect and could cause Gitea to block", q.Name)
		if pausable, ok := returnable.(Pausable); ok {
			log.Warn("Queue: %s is being paused to prevent data-loss, add workers manually and unpause.", q.Name)
			pausable.Pause()
		}
	}

	return returnable
}

// CreateUniqueQueue for name with provided handler and exemplar
func CreateUniqueQueue(name string, handle HandlerFunc, exemplar interface{}) UniqueQueue {
	q, cfg := getQueueSettings(name)
	if len(cfg) == 0 {
		return nil
	}

	if len(q.Type) > 0 && q.Type != "dummy" && q.Type != "immediate" && !strings.HasPrefix(q.Type, "unique-") {
		q.Type = "unique-" + q.Type
	}

	typ, err := validType(q.Type)
	if err != nil || typ == PersistableChannelQueueType {
		typ = PersistableChannelUniqueQueueType
		if err != nil {
			log.Error("Invalid type %s provided for queue named %s defaulting to %s", q.Type, name, string(typ))
		}
	}

	returnable, err := NewQueue(typ, handle, cfg, exemplar)
	if q.WrapIfNecessary && err != nil {
		log.Warn("Unable to create unique queue for %s: %v", name, err)
		log.Warn("Attempting to create wrapped queue")
		returnable, err = NewQueue(WrappedUniqueQueueType, handle, WrappedUniqueQueueConfiguration{
			Underlying:  typ,
			Timeout:     q.Timeout,
			MaxAttempts: q.MaxAttempts,
			Config:      cfg,
			QueueLength: q.QueueLength,
		}, exemplar)
	}
	if err != nil {
		log.Error("Unable to create unique queue for %s: %v", name, err)
		return nil
	}

	// Sanity check configuration
	if q.Workers == 0 && (q.BoostTimeout == 0 || q.BoostWorkers == 0 || q.MaxWorkers == 0) {
		log.Warn("Queue: %s is configured to be non-scaling and have no workers\n - this configuration is likely incorrect and could cause Gitea to block", q.Name)
		if pausable, ok := returnable.(Pausable); ok {
			log.Warn("Queue: %s is being paused to prevent data-loss, add workers manually and unpause.", q.Name)
			pausable.Pause()
		}
	}

	return returnable.(UniqueQueue)
}
