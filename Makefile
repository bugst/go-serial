#
# Copyright 2014 Cristian Maglie. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.
#

include $(GOROOT)/src/Make.inc

TARG=github.com/bugst/go-serial/serial

GOFILES=serial.go native_$(GOOS).go syscall_$(GOOS).go

include $(GOROOT)/src/Make.pkg

