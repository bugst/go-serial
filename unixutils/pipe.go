//
// Copyright 2014-2024 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

//go:build linux || darwin || freebsd || openbsd

package unixutils

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// Pipe represents a unix-pipe
type Pipe struct {
	opened bool
	rd     int
	wr     int
}

// NewPipe creates a new pipe
func NewPipe() (*Pipe, error) {
	fds := []int{0, 0}
	if err := unix.Pipe(fds); err != nil {
		return nil, err
	}
	return &Pipe{
		rd:     fds[0],
		wr:     fds[1],
		opened: true,
	}, nil
}

// ReadFD returns the file handle for the read side of the pipe.
func (p *Pipe) ReadFD() int {
	if !p.opened {
		return -1
	}
	return p.rd
}

// WriteFD returns the file handle for the write side of the pipe.
func (p *Pipe) WriteFD() int {
	if !p.opened {
		return -1
	}
	return p.wr
}

// Write to the pipe the content of data. Returns the number of bytes written.
func (p *Pipe) Write(data []byte) (int, error) {
	if !p.opened {
		return 0, fmt.Errorf("Pipe not opened")
	}
	return unix.Write(p.wr, data)
}

// Read from the pipe into the data array. Returns the number of bytes read.
func (p *Pipe) Read(data []byte) (int, error) {
	if !p.opened {
		return 0, fmt.Errorf("Pipe not opened")
	}
	return unix.Read(p.rd, data)
}

// Close the pipe
func (p *Pipe) Close() error {
	if !p.opened {
		return fmt.Errorf("Pipe not opened")
	}
	err1 := unix.Close(p.rd)
	err2 := unix.Close(p.wr)
	p.opened = false
	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}
	return nil
}
