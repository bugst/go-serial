//
// Copyright 2014-2024 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

//go:build linux || darwin || freebsd || openbsd

package unixutils

import (
	"time"

	"golang.org/x/sys/unix"
)

// FDSet is a set of file descriptors suitable for a select call
type FDSet struct {
	set unix.FdSet
	max int
}

// NewFDSet creates a set of file descriptors suitable for a Select call.
func NewFDSet(fds ...int) *FDSet {
	s := &FDSet{}
	s.Add(fds...)
	return s
}

// Add adds the file descriptors passed as parameter to the FDSet.
func (s *FDSet) Add(fds ...int) {
	for _, fd := range fds {
		s.set.Set(fd)
		if fd > s.max {
			s.max = fd
		}
	}
}

// FDResultSets contains the result of a Select operation.
type FDResultSets struct {
	readable  unix.FdSet
	writeable unix.FdSet
	errors    unix.FdSet
}

// IsReadable test if a file descriptor is ready to be read.
func (r *FDResultSets) IsReadable(fd int) bool {
	return r.readable.IsSet(fd)
}

// IsWritable test if a file descriptor is ready to be written.
func (r *FDResultSets) IsWritable(fd int) bool {
	return r.writeable.IsSet(fd)
}

// IsError test if a file descriptor is in error state.
func (r *FDResultSets) IsError(fd int) bool {
	return r.errors.IsSet(fd)
}

// Select performs a select system call,
// file descriptors in the rd set are tested for read-events,
// file descriptors in the wd set are tested for write-events and
// file descriptors in the er set are tested for error-events.
// The function will block until an event happens or the timeout expires.
// The function return an FDResultSets that contains all the file descriptor
// that have a pending read/write/error event.
func Select(rd, wr, er *FDSet, timeout time.Duration) (FDResultSets, error) {
	max := 0
	res := FDResultSets{}
	if rd != nil {
		res.readable = rd.set
		max = rd.max
	}
	if wr != nil {
		res.writeable = wr.set
		if wr.max > max {
			max = wr.max
		}
	}
	if er != nil {
		res.errors = er.set
		if er.max > max {
			max = er.max
		}
	}

	var err error
	if timeout != -1 {
		t := unix.NsecToTimeval(timeout.Nanoseconds())
		_, err = unix.Select(max+1, &res.readable, &res.writeable, &res.errors, &t)
	} else {
		_, err = unix.Select(max+1, &res.readable, &res.writeable, &res.errors, nil)
	}
	return res, err
}
