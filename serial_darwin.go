//
// Copyright 2014-2016 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package serial // import "go.bug.st/serial.v1"

import "syscall"

const devFolder = "/dev"
const regexFilter = "^(cu|tty)\\..*"

//sys ioctl(fd int, req uint64, data uintptr) (err error)

const ioctlTcgetattr = syscall.TIOCGETA
const ioctlTcsetattr = syscall.TIOCSETA
