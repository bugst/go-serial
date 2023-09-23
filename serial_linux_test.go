//
// Copyright 2014-2023 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

// Testing code idea and fix thanks to @angri
// https://github.com/bugst/go-serial/pull/42

package serial

import (
	"context"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func startSocatAndWaitForPort(t *testing.T, ctx context.Context) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "socat", "-D", "STDIO", "pty,link=/tmp/faketty")
	r, err := cmd.StderrPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	// Let our fake serial port node appear.
	// socat will write to stderr before starting transfer phase;
	// we don't really care what, just that it did, because then it's ready.
	buf := make([]byte, 1024)
	_, err = r.Read(buf)
	require.NoError(t, err)
	return cmd
}

func TestSerialReadAndCloseConcurrency(t *testing.T) {

	// Run this test with race detector to actually test that
	// the correct multitasking behaviour is happening.

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := startSocatAndWaitForPort(t, ctx)
	go cmd.Wait()

	port, err := Open("/tmp/faketty", &Mode{})
	require.NoError(t, err)
	buf := make([]byte, 100)
	go port.Read(buf)
	// let port.Read to start
	time.Sleep(time.Millisecond * 1)
	port.Close()
}

func TestDoubleCloseIsNoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := startSocatAndWaitForPort(t, ctx)
	go cmd.Wait()

	port, err := Open("/tmp/faketty", &Mode{})
	require.NoError(t, err)
	require.NoError(t, port.Close())
	require.NoError(t, port.Close())
}

func TestAccessModeDefault(t *testing.T) {
	AccessModeExclusive(t, &Mode{})
}

func TestAccessModeExclusive(t *testing.T) {
	mode := &Mode{
		AccessMode: Exclusive,
	}

	AccessModeExclusive(t, mode)
}

func AccessModeExclusive(t *testing.T, mode *Mode) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := startSocatAndWaitForPort(t, ctx)
	go cmd.Wait()

	port, err := Open("/tmp/faketty", mode)
	require.NoError(t, err)
	_, err2 := Open("/tmp/faketty", mode)
	require.Error(t, err2, syscall.ENOENT)
	require.NoError(t, port.Close())
}

func TestAccessModeShared(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := startSocatAndWaitForPort(t, ctx)
	go cmd.Wait()

	mode := &Mode{
		AccessMode: Shared,
	}

	port, err := Open("/tmp/faketty", mode)
	require.NoError(t, err)
	port2, err2 := Open("/tmp/faketty", mode)
	require.NoError(t, err2)
	require.NoError(t, port.Close())
	require.NoError(t, port2.Close())
}
