//
// Copyright 2014-2020 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// Testing code idea and fix thanks to @angri
// https://github.com/bugst/go-serial/pull/42
//

package serial

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSerialReadAndCloseConcurrency(t *testing.T) {

	// Run this test with race detector to actually test that
	// the correct multitasking behaviour is happening.

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "socat", "STDIO", "pty,link=/tmp/faketty")
	require.NoError(t, cmd.Start())
	go cmd.Wait()
	// let our fake serial port node to appear
	time.Sleep(time.Millisecond * 100)

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
	cmd := exec.CommandContext(ctx, "socat", "STDIO", "pty,link=/tmp/faketty")
	require.NoError(t, cmd.Start())
	go cmd.Wait()
	// let our fake serial port node to appear
	time.Sleep(time.Millisecond * 100)

	port, err := Open("/tmp/faketty", &Mode{})
	require.NoError(t, err)
	require.NoError(t, port.Close())
	require.NoError(t, port.Close())
}
