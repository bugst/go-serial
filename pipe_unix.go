//
// Copyright 2014-2016 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

// +build linux darwin freebsd

package serial // import "go.bug.st/serial.v1"

import "syscall"

// pipe is a small wrapper around unix-pipe syscall functions
type pipe struct {
	rd int
	wr int
}

func newPipe() (*pipe, error) {
	fds := []int{0, 0}
	if err := syscall.Pipe(fds); err != nil {
		return nil, err
	}
	return &pipe{rd: fds[0], wr: fds[1]}, nil
}

func (p *pipe) ReadFD() int {
	return p.rd
}

func (p *pipe) WriteFD() int {
	return p.wr
}

func (p *pipe) Write(data []byte) (int, error) {
	return syscall.Write(p.wr, data)
}

func (p *pipe) Read(data []byte) (int, error) {
	return syscall.Read(p.rd, data)
}

func (p *pipe) Close() error {
	err1 := syscall.Close(p.rd)
	err2 := syscall.Close(p.wr)
	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}
	return nil
}
