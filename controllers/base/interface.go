package base

import "context"

type Controller interface {
	Run(ctx context.Context, workers int) error
}
