//
// Copyright 2014-2017 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package serial // import "go.bug.st/serial.v1"

//sys regEnumValue(key syscall.Handle, index uint32, name *uint16, nameLen *uint32, reserved *uint32, class *uint16, value *uint16, valueLen *uint32) (regerrno error) = advapi32.RegEnumValueW

const (
	ceBreak    uint32 = 0x0010
	ceFrame           = 0x0008
	ceOverrun         = 0x0002
	ceRxover          = 0x0001
	ceRxparity        = 0x0004
)

type comstat struct {
	/* typedef struct _COMSTAT {
	    DWORD fCtsHold  :1;
	    DWORD fDsrHold  :1;
	    DWORD fRlsdHold  :1;
	    DWORD fXoffHold  :1;
	    DWORD fXoffSent  :1;
	    DWORD fEof  :1;
	    DWORD fTxim  :1;
	    DWORD fReserved  :25;
	    DWORD cbInQue;
	    DWORD cbOutQue;
	} COMSTAT, *LPCOMSTAT; */
	flags  uint32
	inque  uint32
	outque uint32
}

//sys clearCommError(handle syscall.Handle, lpErrors *uint32, lpStat *comstat) (err error) = ClearCommError

const (
	dcbBinary                uint32 = 0x00000001
	dcbParity                       = 0x00000002
	dcbOutXCTSFlow                  = 0x00000004
	dcbOutXDSRFlow                  = 0x00000008
	dcbDTRControlDisableMask        = ^uint32(0x00000030)
	dcbDTRControlEnable             = 0x00000010
	dcbDTRControlHandshake          = 0x00000020
	dcbDSRSensitivity               = 0x00000040
	dcbTXContinueOnXOFF             = 0x00000080
	dcbOutX                         = 0x00000100
	dcbInX                          = 0x00000200
	dcbErrorChar                    = 0x00000400
	dcbNull                         = 0x00000800
	dcbRTSControlDisableMask        = ^uint32(0x00003000)
	dcbRTSControlEnable             = 0x00001000
	dcbRTSControlHandshake          = 0x00002000
	dcbRTSControlToggle             = 0x00003000
	dcbAbortOnError                 = 0x00004000
)

type dcb struct {
	DCBlength uint32
	BaudRate  uint32

	// Flags field is a bitfield
	//  fBinary            :1
	//  fParity            :1
	//  fOutxCtsFlow       :1
	//  fOutxDsrFlow       :1
	//  fDtrControl        :2
	//  fDsrSensitivity    :1
	//  fTXContinueOnXoff  :1
	//  fOutX              :1
	//  fInX               :1
	//  fErrorChar         :1
	//  fNull              :1
	//  fRtsControl        :2
	//  fAbortOnError      :1
	//  fDummy2            :17
	Flags uint32

	wReserved  uint16
	XonLim     uint16
	XoffLim    uint16
	ByteSize   byte
	Parity     byte
	StopBits   byte
	XonChar    byte
	XoffChar   byte
	ErrorChar  byte
	EOFChar    byte
	EvtChar    byte
	wReserved1 uint16
}

//sys getCommState(handle syscall.Handle, dcb *dcb) (err error) = GetCommState

//sys setCommState(handle syscall.Handle, dcb *dcb) (err error) = SetCommState

type commTimeouts struct {
	ReadIntervalTimeout         uint32
	ReadTotalTimeoutMultiplier  uint32
	ReadTotalTimeoutConstant    uint32
	WriteTotalTimeoutMultiplier uint32
	WriteTotalTimeoutConstant   uint32
}

//sys setCommTimeouts(handle syscall.Handle, timeouts *commTimeouts) (err error) = SetCommTimeouts

const (
	commFunctionSetXOFF  = 1
	commFunctionSetXON   = 2
	commFunctionSetRTS   = 3
	commFunctionClrRTS   = 4
	commFunctionSetDTR   = 5
	commFunctionClrDTR   = 6
	commFunctionSetBreak = 8
	commFunctionClrBreak = 9
)

//sys escapeCommFunction(handle syscall.Handle, function uint32) (res bool) = EscapeCommFunction

const (
	msCTSOn  = 0x0010
	msDSROn  = 0x0020
	msRingOn = 0x0040
	msRLSDOn = 0x0080
)

//sys getCommModemStatus(handle syscall.Handle, bits *uint32) (res bool) = GetCommModemStatus

const (
	evBreak   uint32 = 0x0040 // A break was detected on input.
	evCts            = 0x0008 // The CTS (clear-to-send) signal changed state.
	evDsr            = 0x0010 // The DSR (data-set-ready) signal changed state.
	evErr            = 0x0080 // A line-status error occurred. Line-status errors are CE_FRAME, CE_OVERRUN, and CE_RXPARITY.
	evRing           = 0x0100 // A ring indicator was detected.
	evRlsd           = 0x0020 // The RLSD (receive-line-signal-detect) signal changed state.
	evRxChar         = 0x0001 // A character was received and placed in the input buffer.
	evRxFlag         = 0x0002 // The event character was received and placed in the input buffer. The event character is specified in the device's DCB structure, which is applied to a serial port by using the SetCommState function.
	evTxEmpty        = 0x0004 // The last character in the output buffer was sent.
)

//sys setCommMask(handle syscall.Handle, mask uint32) (err error) = SetCommMask

//sys createEvent(eventAttributes *uint32, manualReset bool, initialState bool, name *uint16) (handle syscall.Handle, err error) = CreateEventW

//sys resetEvent(handle syscall.Handle) (err error) = ResetEvent

//sys getOverlappedResult(handle syscall.Handle, overlapEvent *syscall.Overlapped, n *uint32, wait bool) (err error) = GetOverlappedResult

const (
	purgeRxAbort uint32 = 0x0002
	purgeRxClear        = 0x0008
	purgeTxAbort        = 0x0001
	purgeTxClear        = 0x0004
)

//sys purgeComm(handle syscall.Handle, flags uint32) (err error) = PurgeComm
