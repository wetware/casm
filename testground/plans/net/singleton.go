package main

import (
	"context"
	"errors"

	"github.com/wetware/casm/testground/env"
)

func TestBoot(ctx context.Context, env env.Env) error {
	env.RunEnv.RecordSuccess()
	return errors.New("NOT IMPLEMENTED")
}
