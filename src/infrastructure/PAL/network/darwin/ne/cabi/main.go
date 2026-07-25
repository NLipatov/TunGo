//go:build darwin || ios

package main

/*
#include <stdint.h>
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"time"
	"unsafe"

	control "tungo/infrastructure/PAL/network/darwin/ne/internal/controller"
)

var controller = control.New()

//export tungo_network_settings
func tungo_network_settings(output **C.char) *C.char {
	if output == nil {
		return errorString(fmt.Errorf("output network settings pointer is required"))
	}
	*output = nil
	networkSettings, err := controller.NetworkSettings()
	if err != nil {
		return errorString(err)
	}
	payload, err := json.Marshal(networkSettings)
	if err != nil {
		return errorString(err)
	}
	*output = C.CString(string(payload))
	return nil
}

//export tungo_start
func tungo_start(tunnelFileDescriptor C.int32_t) *C.char {
	return errorString(controller.Start(int(tunnelFileDescriptor)))
}

//export tungo_wait_ready
func tungo_wait_ready(timeoutMilliseconds C.int64_t) *C.char {
	timeout := time.Duration(timeoutMilliseconds) * time.Millisecond
	return errorString(controller.WaitReady(timeout))
}

//export tungo_stop
func tungo_stop() *C.char {
	return errorString(controller.Stop())
}

// tungo_free releases memory returned by the TunGo C ABI.
// Passing any other pointer results in undefined behavior.
//
//export tungo_free
func tungo_free(pointer unsafe.Pointer) {
	C.free(pointer)
}

func errorString(err error) *C.char {
	if err == nil {
		return nil
	}
	return C.CString(err.Error())
}

func main() {}
