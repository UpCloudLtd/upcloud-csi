package driver

import (
	"context"
	"fmt"
	"time"

	"github.com/UpCloudLtd/upcloud-go-api/upcloud"
	"golang.org/x/sync/errgroup"
)

// HealthCheck is the interface that must be implemented to be compatible with
// `HealthChecker`.
type HealthCheck interface {
	Name() string
	Check(context.Context) error
}

// HealthChecker helps with writing multi component health checkers.
type HealthChecker struct {
	checks []HealthCheck
}

// NewHealthChecker configures a new health checker with the passed in checks.
func NewHealthChecker(checks ...HealthCheck) *HealthChecker {
	return &HealthChecker{
		checks: checks,
	}
}

// Check runs all configured health checks and return an error if any of the
// checks fail.
func (c *HealthChecker) Check(ctx context.Context) error {
	var eg errgroup.Group

	for _, check := range c.checks {
		eg.Go(func() error {
			return check.Check(ctx)
		})
	}

	return eg.Wait()
}

var upcloudHealthTimeout = 15 * time.Second

type upcloudHealthChecker struct {
	account func() (*upcloud.Account, error)
}

func (c *upcloudHealthChecker) Name() string {
	return "upcloud"
}

func (c *upcloudHealthChecker) Check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, upcloudHealthTimeout)
	defer cancel()
	_, err := c.account()
	if err != nil {
		return fmt.Errorf("checking health: %w", err)
	}
	return nil
}
