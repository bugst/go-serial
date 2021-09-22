package serial

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func openTestPort(t *testing.T) (Port, error) {
	ports, err := GetPortsList()
	if err != nil || len(ports) == 0 {
		t.SkipNow()
	}

	mode := Mode{
		BaudRate: 115200,
		DataBits: 8,
		Parity:   NoParity,
		StopBits: OneStopBit,
	}
	return Open(ports[0], &mode)
}

func TestOpenClose(t *testing.T) {
	// prevent port from being busy in other tests
	defer time.Sleep(time.Millisecond)

	port, err := openTestPort(t)
	require.NoError(t, err)
	port.Close()
}

func TestOpenReadClosed(t *testing.T) {
	// prevent port from being busy in other tests
	defer time.Sleep(time.Millisecond)

	port, err := openTestPort(t)
	require.NoError(t, err)
	defer port.Close()

	done := make(chan struct{})
	var readErr error
	go func() {
		buf := make([]byte, 100)
		_, readErr = port.ReadContext(context.Background(), buf)
		close(done)
	}()

	time.Sleep(time.Millisecond)
	select {
	case <-done:
		require.NoError(t, readErr)
		require.Fail(t, "expected reading to be in-progress")
	default:
	}

	port.Close()

	time.Sleep(time.Millisecond)
	select {
	case <-done:
	default:
		require.Fail(t, "expected reading to be done")
	}

	var portErr *PortError
	if !errors.As(readErr, &portErr) {
		require.Fail(t, "expected read error to be a port error")
	}
	require.Equal(t, portErr.Code(), PortClosed)
}

func TestOpenReadCanceled(t *testing.T) {
	// prevent port from being busy in other tests
	defer time.Sleep(time.Millisecond)

	port, err := openTestPort(t)
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
		require.Fail(t, "expected reading to be done")
	}

	var portErr *PortError
	if !errors.As(readErr, &portErr) {
		require.Fail(t, "expected read error to be a port error")
	}
	require.Equal(t, portErr.Code(), ReadCanceled)
}
