//
// Copyright 2014-2024 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

//go:build !linux && !darwin && !freebsd && !openbsd

package serial

import (
	"errors"
)

func nativeGetPortsList() ([]string, error) {
	return nil, errors.New("nativeGetPortsList is not currently supported on this OS")
}
