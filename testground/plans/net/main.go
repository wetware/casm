package main

import (
	"github.com/testground/sdk-go/run"
	"github.com/wetware/casm/testground/env"
)

func main() {
	run.InvokeMap(map[string]interface{}{
		"boot": env.New(TestBoot),
	})
}
