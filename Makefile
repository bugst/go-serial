include $(GOROOT)/src/Make.inc

TARG=github.com/bugst/go-serial/serial

GOFILES=serial.go native_$(GOOS).go syscall_$(GOOS).go

include $(GOROOT)/src/Make.pkg

