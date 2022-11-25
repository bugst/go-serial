//
// Copyright 2014-2021 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// Testing code idea and fix thanks to @angri
// https://github.com/bugst/go-serial/pull/42
//

package serial

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const ttyPath = "/tmp/faketty"

type ttyProc struct {
	t   *testing.T
	cmd *exec.Cmd
}

func (tp *ttyProc) Close() error {
	err := tp.cmd.Process.Signal(os.Interrupt)
	require.NoError(tp.t, err)
	return tp.cmd.Wait()
}

func (tp *ttyProc) waitForPort() {
	for {
		_, err := os.Stat(ttyPath)
		if err == nil {
			return
		}
		if !errors.Is(err, os.ErrNotExist) {
			require.NoError(tp.t, err)
		}
		time.Sleep(time.Millisecond)
	}
}

func startSocatAndWaitForPort(t *testing.T, ctx context.Context) io.Closer {
	cmd := exec.CommandContext(ctx, "socat", "STDIO", "pty,link="+ttyPath)
	require.NoError(t, cmd.Start())
	socat := &ttyProc{
		t:   t,
		cmd: cmd,
	}
	socat.waitForPort()
	return socat
}

func TestSerialReadAndCloseConcurrency(t *testing.T) {

	// Run this test with race detector to actually test that
	// the correct multitasking behaviour is happening.

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	socat := startSocatAndWaitForPort(t, ctx)
	defer socat.Close()

	port, err := Open(ttyPath, &Mode{})
	require.NoError(t, err)
	defer port.Close()

	buf := make([]byte, 100)
	go port.Read(buf)
	// let port.Read to start
	time.Sleep(time.Millisecond * 1)
}

func TestDoubleCloseIsNoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	socat := startSocatAndWaitForPort(t, ctx)
	defer socat.Close()

	port, err := Open(ttyPath, &Mode{})
	require.NoError(t, err)
	require.NoError(t, port.Close())
	require.NoError(t, port.Close())
}

func TestCancelStopsRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	socat := startSocatAndWaitForPort(t, ctx)
	defer socat.Close()

	port, err := Open(ttyPath, &Mode{})
	require.NoError(t, err)
	defer port.Close()

	readCtx, readCancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var readErr error
	go func() {
		buf := make([]byte, 100)
		_, readErr = port.ReadContext(readCtx, buf)
		close(done)
	}()

	time.Sleep(time.Millisecond)
	select {
	case <-done:
		require.NoError(t, readErr)
		require.Fail(t, "expected reading to be in-progress")
	default:
	}

	readCancel()

	time.Sleep(time.Millisecond)
	select {
	case <-done:
	default:
		require.Fail(t, "expected reading to be finished")

	}

	var portErr *PortError
	if !errors.As(readErr, &portErr) {
		require.Fail(t, "expected read error to be a port error")
	}
	require.Equal(t, portErr.Code(), ReadCanceled)
}
