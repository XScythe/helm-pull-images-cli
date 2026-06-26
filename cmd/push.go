package cmd

import (
	"context"
	"io"

	"helm-deep-pack/internal/push"
	"helm-deep-pack/internal/pushcli"
)

var (
	pushState = pushcli.State{
		Concurrency: 4,
	}
)

var pushRun = push.PushImages

var pushCmd = pushcli.NewCommand(pushcli.Config{
	Use:   "push REGISTRY",
	Short: "Push mirrored images from generated OCI layout artifacts",
	Run: func(ctx context.Context, opts push.Options, status ...io.Writer) error {
		return pushRun(ctx, opts, status...)
	},
	LoggerFactory: commandLogger,
	State:         &pushState,
})
