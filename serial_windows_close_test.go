//go:build windows

package serial

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func testWindowsPort(handle windows.Handle) *windowsPort {
	port := &windowsPort{handle: handle}
	port.liveHandle.Store(uintptr(handle))
	return port
}

func TestWindowsCloseCancelsPendingOverlappedRead(t *testing.T) {
	server, client := connectedOverlappedPipe(t)
	port := testWindowsPort(server)
	pending := make(chan struct{})
	originalRead := readWindowsFile
	readWindowsFile = func(
		handle windows.Handle,
		buffer []byte,
		transferred *uint32,
		overlapped *windows.Overlapped,
	) error {
		err := originalRead(handle, buffer, transferred, overlapped)
		if errors.Is(err, windows.ERROR_IO_PENDING) {
			close(pending)
		}
		return err
	}
	defer func() { readWindowsFile = originalRead }()

	readDone := make(chan error, 1)
	go func() {
		buffer := make([]byte, 1)
		_, err := port.Read(buffer)
		readDone <- err
	}()

	// The peer deliberately sends no data. Wait until ReadFile has explicitly
	// reported ERROR_IO_PENDING before closing from another goroutine.
	select {
	case <-pending:
	case <-time.After(time.Second):
		t.Fatal("ReadFile did not enter the pending overlapped state")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- port.Close() }()

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("close pending read: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not return after cancelling a pending overlapped read")
	}

	select {
	case err := <-readDone:
		var portErr *PortError
		if !errors.As(err, &portErr) || portErr.Code() != PortClosed {
			t.Fatalf("read error = %v, want PortClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pending Read did not return after Close")
	}

	var duplicate windows.Handle
	duplicateErr := windows.DuplicateHandle(
		windows.CurrentProcess(), server,
		windows.CurrentProcess(), &duplicate,
		0, false, windows.DUPLICATE_SAME_ACCESS,
	)
	if duplicateErr == nil {
		windows.CloseHandle(duplicate)
		t.Fatal("serial handle remained valid after Close")
	}
	if !errors.Is(duplicateErr, windows.ERROR_INVALID_HANDLE) {
		t.Fatalf("duplicate closed serial handle: %v, want ERROR_INVALID_HANDLE", duplicateErr)
	}

	if err := windows.CloseHandle(client); err != nil {
		t.Fatalf("close pipe peer: %v", err)
	}
}

func TestWindowsCloseCancelsReadBlockedInsideOverlappedIssuance(t *testing.T) {
	const handle = windows.Handle(1)
	port := testWindowsPort(handle)
	readEntered := make(chan struct{})
	releaseRead := make(chan struct{})
	var releaseOnce sync.Once
	var cancelCalls atomic.Int32
	var closeCalls atomic.Int32
	originalRead := readWindowsFile
	originalCancel := cancelPendingIO
	originalClose := closeWindowsHandle
	readWindowsFile = func(
		windows.Handle,
		[]byte,
		*uint32,
		*windows.Overlapped,
	) error {
		close(readEntered)
		<-releaseRead
		return windows.ERROR_OPERATION_ABORTED
	}
	cancelPendingIO = func(got windows.Handle, _ *windows.Overlapped) error {
		if got != handle {
			t.Errorf("CancelIoEx handle = %v, want %v", got, handle)
		}
		cancelCalls.Add(1)
		releaseOnce.Do(func() { close(releaseRead) })
		return nil
	}
	closeWindowsHandle = func(got windows.Handle) error {
		if got != handle {
			t.Errorf("CloseHandle handle = %v, want %v", got, handle)
		}
		closeCalls.Add(1)
		return nil
	}
	defer func() {
		readWindowsFile = originalRead
		cancelPendingIO = originalCancel
		closeWindowsHandle = originalClose
	}()

	readDone := make(chan error, 1)
	go func() {
		_, err := port.Read(make([]byte, 1))
		readDone <- err
	}()
	select {
	case <-readEntered:
	case <-time.After(time.Second):
		t.Fatal("ReadFile did not block inside overlapped issuance")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- port.Close() }()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close waited on the issuance mutex before cancelling blocked ReadFile")
	}
	select {
	case err := <-readDone:
		var portErr *PortError
		if !errors.As(err, &portErr) || portErr.Code() != PortClosed {
			t.Fatalf("read error = %v, want PortClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked ReadFile did not return after Close cancellation")
	}
	if calls := cancelCalls.Load(); calls < 2 {
		t.Fatalf("CancelIoEx calls = %d, want pre-lock and final cancellation", calls)
	}
	if calls := closeCalls.Load(); calls != 1 {
		t.Fatalf("CloseHandle calls = %d, want 1", calls)
	}
}

func TestWindowsCloseCancelsWriteBlockedInsideOverlappedIssuance(t *testing.T) {
	const handle = windows.Handle(1)
	port := testWindowsPort(handle)
	writeEntered := make(chan struct{})
	releaseWrite := make(chan struct{})
	var releaseOnce sync.Once
	var cancelCalls atomic.Int32
	var closeCalls atomic.Int32
	originalWrite := writeWindowsFile
	originalCancel := cancelPendingIO
	originalClose := closeWindowsHandle
	writeWindowsFile = func(
		windows.Handle,
		[]byte,
		*uint32,
		*windows.Overlapped,
	) error {
		close(writeEntered)
		<-releaseWrite
		return windows.ERROR_OPERATION_ABORTED
	}
	cancelPendingIO = func(got windows.Handle, _ *windows.Overlapped) error {
		if got != handle {
			t.Errorf("CancelIoEx handle = %v, want %v", got, handle)
		}
		cancelCalls.Add(1)
		releaseOnce.Do(func() { close(releaseWrite) })
		return nil
	}
	closeWindowsHandle = func(got windows.Handle) error {
		if got != handle {
			t.Errorf("CloseHandle handle = %v, want %v", got, handle)
		}
		closeCalls.Add(1)
		return nil
	}
	defer func() {
		writeWindowsFile = originalWrite
		cancelPendingIO = originalCancel
		closeWindowsHandle = originalClose
	}()

	writeDone := make(chan error, 1)
	go func() {
		_, err := port.Write([]byte{1})
		writeDone <- err
	}()
	select {
	case <-writeEntered:
	case <-time.After(time.Second):
		t.Fatal("WriteFile did not block inside overlapped issuance")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- port.Close() }()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close waited on the issuance mutex before cancelling blocked WriteFile")
	}
	select {
	case err := <-writeDone:
		var portErr *PortError
		if !errors.As(err, &portErr) || portErr.Code() != PortClosed {
			t.Fatalf("write error = %v, want PortClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked WriteFile did not return after Close cancellation")
	}
	if calls := cancelCalls.Load(); calls < 2 {
		t.Fatalf("CancelIoEx calls = %d, want pre-lock and final cancellation", calls)
	}
	if calls := closeCalls.Load(); calls != 1 {
		t.Fatalf("CloseHandle calls = %d, want 1", calls)
	}
}

func TestWindowsCloseBoundsBlockedIssuanceAndLaterRetries(t *testing.T) {
	const handle = windows.Handle(1)
	port := testWindowsPort(handle)
	readEntered := make(chan struct{})
	releaseRead := make(chan struct{})
	var releaseOnce sync.Once
	var allowCancellation atomic.Bool
	var closeCalls atomic.Int32
	originalRead := readWindowsFile
	originalCancel := cancelPendingIO
	originalClose := closeWindowsHandle
	originalTimeout := closeIssueLockTimeout
	originalDelay := closeIssueRetryDelay
	readWindowsFile = func(
		windows.Handle,
		[]byte,
		*uint32,
		*windows.Overlapped,
	) error {
		close(readEntered)
		<-releaseRead
		return windows.ERROR_OPERATION_ABORTED
	}
	cancelPendingIO = func(windows.Handle, *windows.Overlapped) error {
		if allowCancellation.Load() {
			releaseOnce.Do(func() { close(releaseRead) })
		}
		return nil
	}
	closeWindowsHandle = func(windows.Handle) error {
		closeCalls.Add(1)
		return nil
	}
	closeIssueLockTimeout = 25 * time.Millisecond
	closeIssueRetryDelay = time.Millisecond
	defer func() {
		readWindowsFile = originalRead
		cancelPendingIO = originalCancel
		closeWindowsHandle = originalClose
		closeIssueLockTimeout = originalTimeout
		closeIssueRetryDelay = originalDelay
	}()

	readDone := make(chan error, 1)
	go func() {
		_, err := port.Read(make([]byte, 1))
		readDone <- err
	}()
	select {
	case <-readEntered:
	case <-time.After(time.Second):
		t.Fatal("ReadFile did not block inside overlapped issuance")
	}

	started := time.Now()
	err := port.Close()
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("first Close error = %v, want bounded issuance timeout", err)
	}
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("first Close took %s, want bounded return", elapsed)
	}
	if got := windows.Handle(port.liveHandle.Load()); got != handle || port.handle != handle {
		t.Fatalf("failed Close lost handle ownership: live=%v handle=%v want %v", got, port.handle, handle)
	}
	if calls := closeCalls.Load(); calls != 0 {
		t.Fatalf("CloseHandle calls after issuance timeout = %d, want 0", calls)
	}

	allowCancellation.Store(true)
	if err := port.Close(); err != nil {
		t.Fatalf("retry Close: %v", err)
	}
	select {
	case err := <-readDone:
		var portErr *PortError
		if !errors.As(err, &portErr) || portErr.Code() != PortClosed {
			t.Fatalf("read error = %v, want PortClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked ReadFile did not return after retry Close")
	}
	if calls := closeCalls.Load(); calls != 1 {
		t.Fatalf("CloseHandle calls after retry = %d, want 1", calls)
	}
}

func TestWindowsCloseReleasesHandleBeforeJoiningDriverCancellation(t *testing.T) {
	port := testWindowsPort(windows.Handle(1))
	readStarted := make(chan struct{})
	handleClosed := make(chan struct{})
	waiterSawClose := make(chan struct{})
	releaseWaiter := make(chan struct{})
	originalRead := readWindowsFile
	originalEventWait := waitForOverlappedEvent
	originalResultWait := waitForOverlappedResult
	originalCancel := cancelPendingIO
	originalClose := closeWindowsHandle
	var resultCalls atomic.Int32
	readWindowsFile = func(
		windows.Handle,
		[]byte,
		*uint32,
		*windows.Overlapped,
	) error {
		close(readStarted)
		return windows.ERROR_IO_PENDING
	}
	waitForOverlappedEvent = func(windows.Handle, uint32) (uint32, error) {
		// Model the CH340 behavior observed on Cafe: CancelIoEx is accepted,
		// but completion is not delivered until CloseHandle releases the device.
		<-handleClosed
		close(waiterSawClose)
		<-releaseWaiter
		return windows.WAIT_OBJECT_0, nil
	}
	waitForOverlappedResult = func(
		windows.Handle,
		*windows.Overlapped,
		*uint32,
		bool,
	) error {
		resultCalls.Add(1)
		return windows.ERROR_INVALID_HANDLE
	}
	cancelPendingIO = func(windows.Handle, *windows.Overlapped) error { return nil }
	closeWindowsHandle = func(windows.Handle) error {
		close(handleClosed)
		return nil
	}
	defer func() {
		readWindowsFile = originalRead
		waitForOverlappedEvent = originalEventWait
		waitForOverlappedResult = originalResultWait
		cancelPendingIO = originalCancel
		closeWindowsHandle = originalClose
	}()

	readDone := make(chan error, 1)
	go func() {
		_, err := port.Read(make([]byte, 1))
		readDone <- err
	}()
	select {
	case <-readStarted:
	case <-time.After(time.Second):
		t.Fatal("ReadFile was not issued")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- port.Close() }()
	select {
	case <-waiterSawClose:
	case <-time.After(time.Second):
		t.Fatal("Close did not release the handle before joining the waiter")
	}
	select {
	case err := <-closeDone:
		t.Fatalf("Close returned before the waiter released its OVERLAPPED and buffer: %v", err)
	default:
	}
	close(releaseWaiter)
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not finish after the waiter released its resources")
	}
	select {
	case err := <-readDone:
		var portErr *PortError
		if !errors.As(err, &portErr) || portErr.Code() != PortClosed {
			t.Fatalf("read error = %v, want PortClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("read did not finish after handle release")
	}
	if calls := resultCalls.Load(); calls != 0 {
		t.Fatalf("GetOverlappedResult calls after handle close = %d, want 0", calls)
	}
}

func TestWindowsCloseReleasesHandleBeforeJoiningDriverWriteCancellation(t *testing.T) {
	port := testWindowsPort(windows.Handle(1))
	writeStarted := make(chan struct{})
	handleClosed := make(chan struct{})
	releaseWaiter := make(chan struct{})
	originalWrite := writeWindowsFile
	originalEventWait := waitForOverlappedEvent
	originalResultWait := waitForOverlappedResult
	originalCancel := cancelPendingIO
	originalClose := closeWindowsHandle
	var resultCalls atomic.Int32
	writeWindowsFile = func(
		windows.Handle,
		[]byte,
		*uint32,
		*windows.Overlapped,
	) error {
		close(writeStarted)
		return windows.ERROR_IO_PENDING
	}
	waitForOverlappedEvent = func(windows.Handle, uint32) (uint32, error) {
		<-handleClosed
		<-releaseWaiter
		return windows.WAIT_OBJECT_0, nil
	}
	waitForOverlappedResult = func(
		windows.Handle,
		*windows.Overlapped,
		*uint32,
		bool,
	) error {
		resultCalls.Add(1)
		return windows.ERROR_INVALID_HANDLE
	}
	cancelPendingIO = func(windows.Handle, *windows.Overlapped) error { return nil }
	closeWindowsHandle = func(windows.Handle) error {
		close(handleClosed)
		return nil
	}
	defer func() {
		writeWindowsFile = originalWrite
		waitForOverlappedEvent = originalEventWait
		waitForOverlappedResult = originalResultWait
		cancelPendingIO = originalCancel
		closeWindowsHandle = originalClose
	}()

	writeDone := make(chan error, 1)
	go func() {
		_, err := port.Write([]byte{1})
		writeDone <- err
	}()
	select {
	case <-writeStarted:
	case <-time.After(time.Second):
		t.Fatal("WriteFile was not issued")
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- port.Close() }()
	select {
	case <-handleClosed:
	case <-time.After(time.Second):
		t.Fatal("Close did not release the handle before joining the write waiter")
	}
	close(releaseWaiter)
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not finish after the write waiter released its resources")
	}
	select {
	case err := <-writeDone:
		var portErr *PortError
		if !errors.As(err, &portErr) || portErr.Code() != PortClosed {
			t.Fatalf("write error = %v, want PortClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("write did not finish after handle release")
	}
	if calls := resultCalls.Load(); calls != 0 {
		t.Fatalf("GetOverlappedResult calls after handle close = %d, want 0", calls)
	}
}

func TestWindowsReadUsesNonblockingResultOnlyWhileHandleIsLive(t *testing.T) {
	const handle = windows.Handle(1)
	port := testWindowsPort(handle)
	originalRead := readWindowsFile
	originalEventWait := waitForOverlappedEvent
	originalResultWait := waitForOverlappedResult
	readWindowsFile = func(
		windows.Handle,
		[]byte,
		*uint32,
		*windows.Overlapped,
	) error {
		return windows.ERROR_IO_PENDING
	}
	waitForOverlappedEvent = func(windows.Handle, uint32) (uint32, error) {
		return windows.WAIT_OBJECT_0, nil
	}
	waitForOverlappedResult = func(
		gotHandle windows.Handle,
		_ *windows.Overlapped,
		transferred *uint32,
		wait bool,
	) error {
		if gotHandle != handle {
			t.Errorf("GetOverlappedResult handle = %v, want %v", gotHandle, handle)
		}
		if wait {
			t.Error("GetOverlappedResult used a blocking handle wait after event completion")
		}
		*transferred = 1
		return nil
	}
	defer func() {
		readWindowsFile = originalRead
		waitForOverlappedEvent = originalEventWait
		waitForOverlappedResult = originalResultWait
	}()

	buffer := make([]byte, 1)
	read, err := port.Read(buffer)
	if err != nil || read != 1 {
		t.Fatalf("Read = %d, %v; want 1, nil", read, err)
	}
}

func TestWindowsCloseReturnsCancellationErrorAndLaterCloseRetries(t *testing.T) {
	server, client := connectedOverlappedPipe(t)
	port := testWindowsPort(server)
	cancelErr := windows.ERROR_ACCESS_DENIED
	pending := make(chan struct{})
	originalRead := readWindowsFile
	originalCancel := cancelPendingIO
	originalClose := closeWindowsHandle
	cancelCalls := 0
	var closeCalls atomic.Int32
	readWindowsFile = func(
		handle windows.Handle,
		buffer []byte,
		transferred *uint32,
		overlapped *windows.Overlapped,
	) error {
		err := originalRead(handle, buffer, transferred, overlapped)
		if errors.Is(err, windows.ERROR_IO_PENDING) {
			close(pending)
		}
		return err
	}
	cancelPendingIO = func(handle windows.Handle, overlapped *windows.Overlapped) error {
		cancelCalls++
		if cancelCalls == 1 {
			return cancelErr
		}
		return originalCancel(handle, overlapped)
	}
	closeWindowsHandle = func(handle windows.Handle) error {
		closeCalls.Add(1)
		return originalClose(handle)
	}
	defer func() {
		readWindowsFile = originalRead
		cancelPendingIO = originalCancel
		closeWindowsHandle = originalClose
	}()

	readDone := make(chan error, 1)
	go func() {
		buffer := make([]byte, 1)
		_, err := port.Read(buffer)
		readDone <- err
	}()
	select {
	case <-pending:
	case <-time.After(time.Second):
		t.Fatal("ReadFile did not enter the pending overlapped state")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- port.Close() }()
	select {
	case err := <-closeDone:
		if !errors.Is(err, cancelErr) {
			t.Fatalf("first Close error = %v, want cancellation error", err)
		}
	case <-time.After(time.Second):
		t.Fatal("first Close did not return after cancellation failed")
	}
	if calls := closeCalls.Load(); calls != 0 {
		t.Fatalf("CloseHandle calls after failed cancellation = %d, want 0", calls)
	}
	if port.handle != server {
		t.Fatalf("handle = %v after failed cancellation, want retained %v", port.handle, server)
	}
	select {
	case err := <-readDone:
		t.Fatalf("pending read returned after failed cancellation: %v", err)
	default:
	}

	var duplicate windows.Handle
	if err := windows.DuplicateHandle(
		windows.CurrentProcess(), server,
		windows.CurrentProcess(), &duplicate,
		0, false, windows.DUPLICATE_SAME_ACCESS,
	); err != nil {
		t.Fatalf("duplicate retained serial handle: %v", err)
	}
	if err := windows.CloseHandle(duplicate); err != nil {
		t.Fatalf("close duplicate retained handle: %v", err)
	}

	// A later Close retries cancellation. This time the real CancelIoEx runs,
	// the waiter observes ERROR_OPERATION_ABORTED, and only then is the handle
	// released.
	go func() { closeDone <- port.Close() }()
	select {
	case err := <-readDone:
		var portErr *PortError
		if !errors.As(err, &portErr) || portErr.Code() != PortClosed {
			t.Fatalf("read error = %v, want PortClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pending read did not return after retrying Close")
	}

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("retry Close: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("retry Close did not finish after cancellation succeeded")
	}
	if calls := closeCalls.Load(); cancelCalls < 3 || calls != 1 {
		t.Fatalf("calls = cancel %d, close %d; want failed pre-lock plus at least pre-lock/final retry cancellation and one close", cancelCalls, calls)
	}
	if port.handle != 0 {
		t.Fatalf("handle = %v after completed close, want 0", port.handle)
	}
	if repeatedErr := port.Close(); repeatedErr != nil {
		t.Fatalf("repeated Close after successful retry = %v, want nil", repeatedErr)
	}
	if err := windows.CloseHandle(client); err != nil {
		t.Fatalf("close pipe peer: %v", err)
	}
}

func TestWindowsOperationAbortedMapsToPortClosed(t *testing.T) {
	err := windowsIOError(windows.ERROR_OPERATION_ABORTED)
	var portErr *PortError
	if !errors.As(err, &portErr) || portErr.Code() != PortClosed {
		t.Fatalf("error = %v, want PortClosed", err)
	}
}

func connectedOverlappedPipe(t *testing.T) (windows.Handle, windows.Handle) {
	t.Helper()
	name, err := windows.UTF16PtrFromString(fmt.Sprintf(
		`\\.\pipe\go-serial-close-%d-%d`, os.Getpid(), time.Now().UnixNano(),
	))
	if err != nil {
		t.Fatalf("pipe name: %v", err)
	}

	server, err := windows.CreateNamedPipe(
		name,
		windows.PIPE_ACCESS_INBOUND|windows.FILE_FLAG_OVERLAPPED,
		windows.PIPE_TYPE_BYTE|windows.PIPE_WAIT,
		1,
		4096,
		4096,
		0,
		nil,
	)
	if err != nil {
		t.Fatalf("create named pipe: %v", err)
	}

	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		windows.CloseHandle(server)
		t.Fatalf("create connect event: %v", err)
	}
	defer windows.CloseHandle(event)
	overlapped := &windows.Overlapped{HEvent: event}
	connectErr := windows.ConnectNamedPipe(server, overlapped)
	if connectErr != nil && !errors.Is(connectErr, windows.ERROR_IO_PENDING) {
		windows.CloseHandle(server)
		t.Fatalf("connect named pipe: %v", connectErr)
	}

	client, err := windows.CreateFile(
		name,
		windows.GENERIC_WRITE,
		0,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		windows.CloseHandle(server)
		t.Fatalf("open named pipe peer: %v", err)
	}

	if errors.Is(connectErr, windows.ERROR_IO_PENDING) {
		var transferred uint32
		if err := windows.GetOverlappedResult(server, overlapped, &transferred, true); err != nil {
			windows.CloseHandle(client)
			windows.CloseHandle(server)
			t.Fatalf("finish named pipe connection: %v", err)
		}
	}
	return server, client
}
