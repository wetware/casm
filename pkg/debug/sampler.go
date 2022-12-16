package debug

import (
	"bytes"
	"context"
	"errors"
	"io"
	"runtime/pprof"
	"runtime/trace"
	"time"

	"capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/exp/clock"
	"capnproto.org/go/capnp/v3/flowcontrol/bbr"
	api "github.com/wetware/casm/internal/api/debug"
	casm "github.com/wetware/casm/pkg"
	"github.com/wetware/casm/pkg/util/stream"
)

var ErrNoStrategy = errors.New("no strategy")

type SamplingStrategy interface {
	Start(io.Writer) error
	Stop()
}

type Trace struct{}

func (Trace) Start(w io.Writer) error {
	return trace.Start(w)
}

func (Trace) Stop() {
	trace.Stop()
}

type SampleCPU struct{}

func (SampleCPU) Start(w io.Writer) error {
	return pprof.StartCPUProfile(w)
}

func (SampleCPU) Stop() {
	pprof.StopCPUProfile()
}

type Sampler api.Sampler

func (s Sampler) AddRef() Sampler {
	return Sampler(capnp.Client(s).AddRef())
}

func (s Sampler) Release() {
	capnp.Client(s).Release()
}

// Sample collects samples for duration d, and streams them to the writer.
// If d <= 0, a default value of 1s is used.
func (s Sampler) Sample(ctx context.Context, w io.Writer, d time.Duration) error {
	bind := func(ps api.Sampler_sample_Params) error {
		ps.SetDuration(int64(d))

		server := writeServer{Writer: w}
		writer := api.Sampler_Writer_ServerToClient(server)
		return ps.SetWriter(writer)
	}

	f, release := api.Sampler(s).Sample(ctx, bind)
	defer release()

	return casm.Future(f).Err()
}

// SamplingServer is a profiler that repeatedly samples the process
// for a length of time determined by the caller.
type SamplingServer struct {
	Strategy SamplingStrategy
}

func (s SamplingServer) Sampler() Sampler {
	return Sampler(api.Sampler_ServerToClient(s))
}

func (s SamplingServer) Client() capnp.Client {
	return capnp.Client(s.Sampler())
}

func (s SamplingServer) Sample(ctx context.Context, call api.Sampler_sample) (err error) {
	if s.Strategy == nil {
		return ErrNoStrategy
	}

	w := writer(ctx, call)
	if err = s.sample(ctx, call, w); err == nil {
		err = w.Wait()
	}

	return
}

func (s SamplingServer) sample(ctx context.Context, call api.Sampler_sample, w streamWriter) error {
	if err := s.Strategy.Start(w); err != nil {
		return err
	}
	defer s.Strategy.Stop()

	select {
	case <-time.After(duration(call)):
	case <-ctx.Done():
	}

	return ctx.Err()
}

func duration(call api.Sampler_sample) time.Duration {
	if d := time.Duration(call.Args().Duration()); d > 0 {
		return d
	}

	return time.Second
}

/*
	Stream
*/

type streamWriter struct {
	ctx    context.Context
	stream *stream.Stream[api.Sampler_Writer_write_Params]
}

func writer(ctx context.Context, call api.Sampler_sample) streamWriter {
	w := call.Args().Writer()
	w.SetFlowLimiter(bbr.NewLimiter(clock.System))

	return streamWriter{
		ctx:    ctx,
		stream: stream.New(w.Write),
	}
}

func (s streamWriter) Write(b []byte) (int, error) {
	s.stream.Call(s.ctx, func(s api.Sampler_Writer_write_Params) error {
		return s.SetSample(b)
	})

	if !s.stream.Open() {
		return 0, s.stream.Wait()
	}

	return len(b), nil
}

func (s streamWriter) Wait() error {
	return s.stream.Wait()
}

/*
	sample writer
*/

type writeServer struct {
	io.Writer
}

func (ws writeServer) Write(_ context.Context, call api.Sampler_Writer_write) error {
	b, err := call.Args().Sample()
	if err == nil {
		_, err = io.Copy(ws.Writer, bytes.NewReader(b))
	}
	return err
}
