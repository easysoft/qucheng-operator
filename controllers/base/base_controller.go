package base

import (
	"context"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
	"sync"
	"time"
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
