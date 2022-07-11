// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package base

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
)

type GenericController struct {
	name        string
	Queue       workqueue.RateLimitingInterface
	Logger      logrus.FieldLogger
	SyncHandler func(key string) error
}

func NewGenericController(name string, logger logrus.FieldLogger) *GenericController {
	c := &GenericController{
		name:   name,
		Queue:  workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), name),
		Logger: logger.WithField("controller", name),
	}

	return c
}

func (c *GenericController) Run(ctx context.Context, numWorkers int) error {
	if c.SyncHandler == nil {
		panic("syncHandler required")
	}

	var wg sync.WaitGroup

	defer func() {
		c.Logger.Info("Waiting for workers to finish their work")

		c.Queue.ShutDown()

		wg.Wait()

		c.Logger.Info("All workers have finished")

	}()

	if c.SyncHandler != nil {
		wg.Add(numWorkers)
		for i := 0; i < numWorkers; i++ {
			go func() {
				wait.Until(c.work, time.Second, ctx.Done())
				wg.Done()
			}()
		}
	}

	<-ctx.Done()

	return nil
}

func (c *GenericController) work() {
	for c.processNextItem() {

	}
}

func (c *GenericController) processNextItem() bool {
	key, quit := c.Queue.Get()
	if quit {
		return false
	}

	c.Logger.Infof("process for key %s", key)
	defer c.Queue.Done(key)

	err := c.SyncHandler(key.(string))
	if err != nil {
		c.Queue.AddRateLimited(key)
		return false
	}

	c.Queue.Forget(key)
	return true
}
