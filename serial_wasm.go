//
// Copyright 2014-2024 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package serial

import (
	"errors"
)

func nativeOpen(portName string, mode *Mode) (Port, error) {
	return nil, errors.New("nativeOpen is not supported on wasm")
}
