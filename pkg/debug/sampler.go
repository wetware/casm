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
	api "github.com/wetware/casm/internal/api/debug"
	casm "github.com/wetware/casm/pkg"
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

func (s SamplingServer) Sample(ctx context.Context, call api.Sampler_sample) error {
	if s.Strategy == nil {
		return ErrNoStrategy
	}

	c, err := s.sample(ctx, call)
	if err == nil {
		err = c.Close()
	}

	return err
}

func (s SamplingServer) sample(ctx context.Context, call api.Sampler_sample) (io.Closer, error) {
	w := samplerWriter{
		ctx:            ctx,
		Sampler_Writer: call.Args().Writer(),
	}

	if err := s.Strategy.Start(w); err != nil {
		return nil, err
	}
	defer s.Strategy.Stop()

	select {
	case <-time.After(duration(call)):
	case <-ctx.Done():
	}

	return w, ctx.Err()
}

func duration(call api.Sampler_sample) time.Duration {
	if d := time.Duration(call.Args().Duration()); d > 0 {
		return d
	}

	return time.Second
}

/*
	sample writer
*/

type samplerWriter struct {
	ctx context.Context
	api.Sampler_Writer
}

func (w samplerWriter) Close() error {
	return w.WaitStreaming()
}

func (w samplerWriter) Write(b []byte) (int, error) {
	err := w.Sampler_Writer.Write(w.ctx, func(ps api.Sampler_Writer_write_Params) error {
		return ps.SetSample(b)
	})
	return len(b), err
}

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
