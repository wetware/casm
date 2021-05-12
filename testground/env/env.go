// Package env exposes a thin abstraction over Testground's Go SDK to standardize the test envionment for CASM.
package env

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p/config"
	"github.com/lthibault/log"
	zaputil "github.com/lthibault/log/util/zap"
	"github.com/testground/sdk-go/run"
	"github.com/testground/sdk-go/runtime"
	"github.com/wetware/casm/pkg/net"
)

// Env contains runtime configuration and factory methods for the test environment.
type Env struct {
	Log  log.Logger
	Rand *rand.Rand

	RunEnv  *runtime.RunEnv
	InitCtx *run.InitContext

	Host      host.Host
	Discovery DiscoveryClient
	Overlay   *net.Overlay
}

func (env *Env) HostAddr() peer.AddrInfo { return *host.InfoFromHost(env.Host) }

// TestFunc implements a test case.
type TestFunc func(context.Context, Env) error

// New test case.
func New(f TestFunc) run.InitializedTestCaseFn {
	return func(runEnv *runtime.RunEnv, initCtx *run.InitContext) error {
		ctx, cancel := newContext(runEnv)
		defer cancel()

		env := Env{RunEnv: runEnv, InitCtx: initCtx}
		for _, apply := range []factory{
			newLoggerFactory(),
			newRandFactory(),
			newDiscoveryFactory(),
			newHostFactory(ctx),
			newOverlayFactory(ctx),
		} {
			if err := apply(&env); err != nil {
				return fmt.Errorf("new env: %w", err)
			}
		}

		return f(ctx, env)
	}
}

type factory func(*Env) error

func newLoggerFactory() factory {
	return func(env *Env) error {
		env.Log = zaputil.Wrap(env.RunEnv.SLogger())
		return nil
	}
}

func newRandFactory() factory {
	src := rand.NewSource(time.Now().UnixNano())

	return func(env *Env) error {
		if i := env.RunEnv.IntParam("seed"); i > 0 {
			src.Seed(int64(i))
		}

		env.Rand = rand.New(&atomicSource{Source: src})
		return nil
	}
}

func newHostFactory(ctx context.Context) factory {
	opt := []config.Option{
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	}

	return func(env *Env) (err error) {
		env.Host, err = libp2p.New(ctx, opt...)
		return
	}
}

func newDiscoveryFactory() factory {
	return func(env *Env) error {
		env.Discovery = DiscoveryClient{
			c:   env.InitCtx.SyncClient,
			r:   env.Rand,
			i:   env.RunEnv.TestGroupInstanceCount,
			log: env.Log.WithField("locus", "discovery"),
		}
		return nil
	}
}

func newOverlayFactory(ctx context.Context) factory {
	return func(env *Env) (err error) {
		ctx, cancel := context.WithTimeout(ctx, time.Second*15)
		defer cancel()

		env.Overlay, err = net.New(env.Host,
			net.WithLogger(net.Logger{Logger: env.Log}),
			net.WithNamespace("casm.test.net"),
			net.WithRand(env.Rand))

		if err == nil {
			env.Overlay.Join(ctx, env.Discovery, WithLimit(env.RunEnv))
		}

		return
	}
}

func newContext(runEnv *runtime.RunEnv) (context.Context, context.CancelFunc) {
	ctx, cancel := context.Background(), func() {}

	if i := runEnv.IntParam("timeout"); i > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Second*time.Duration(i))
	}

	return ctx, cancel
}

type atomicSource struct {
	sync.Mutex
	rand.Source
}

func (src *atomicSource) Int63() int64 {
	src.Lock()
	defer src.Unlock()
	return src.Source.Int63()
}

func (src *atomicSource) Seed(s int64) {
	src.Lock()
	src.Source.Seed(s)
	src.Unlock()
}
