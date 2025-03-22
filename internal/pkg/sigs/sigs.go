package sigs

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog/log"
)

func InitSignals() context.Context {
	ctx := handleKill(context.Background())
	return ctx
}

// Handle signal by cancelling children of 'ctx'.
// Signal set may be specified, defaults to  [SIGTERM, SIGINT].
func handleKill(ctx context.Context, sigs ...os.Signal) context.Context {
	ctx, cfunc := context.WithCancel(ctx)

	if len(sigs) == 0 {
		sigs = []os.Signal{syscall.SIGTERM, syscall.SIGINT}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, sigs...)

	go func() {
		select {
		case s := <-sigCh:
			log.Info().
				Str("signal", s.String()).
				Msg("Signal received")
			cfunc()
		case <-ctx.Done():
			log.Info().
				Err(ctx.Err()).
				Msg("Exit signal handler on ctx.Done()")
		}
		signal.Stop(sigCh)
	}()

	return ctx
}
