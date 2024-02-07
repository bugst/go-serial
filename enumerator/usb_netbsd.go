//
// Copyright 2014-2023 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package enumerator

import (
	"go.bug.st/serial"
)

func nativeGetDetailedPortsList() ([]*PortDetails, error) {
	// Retrieve the port list
	ports, err := serial.GetPortsList()
	if err != nil {
		return nil, &PortEnumerationError{causedBy: err}
	}

	var res []*PortDetails
	for _, port := range ports {
		res = append(res, &PortDetails{Name: port})
	}
	return res, nil
}
