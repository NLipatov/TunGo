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
	"runtime/cgo"
	"time"
	"unsafe"

	"tungo/infrastructure/PAL/network/darwin/ne"
	control "tungo/infrastructure/PAL/network/darwin/ne/internal/controller"
)

//export tungo_controller_create
func tungo_controller_create() C.uintptr_t {
	return C.uintptr_t(cgo.NewHandle(control.New()))
}

//export tungo_controller_destroy
func tungo_controller_destroy(controllerHandle C.uintptr_t) {
	releaseControllerHandle(cgo.Handle(controllerHandle))
}

//export tungo_network_settings
func tungo_network_settings(output **C.char) *C.char {
	if output == nil {
		return errorString(fmt.Errorf("output network settings pointer is required"))
	}
	*output = nil
	networkSettings, err := ne.LoadSettings()
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
func tungo_start(controllerHandle C.uintptr_t, tunnelFileDescriptor C.int32_t) *C.char {
	controller, err := resolveController(cgo.Handle(controllerHandle))
	if err != nil {
		return errorString(err)
	}
	return errorString(controller.Start(int(tunnelFileDescriptor)))
}

//export tungo_wait_ready
func tungo_wait_ready(controllerHandle C.uintptr_t, timeoutMilliseconds C.int64_t) *C.char {
	controller, err := resolveController(cgo.Handle(controllerHandle))
	if err != nil {
		return errorString(err)
	}
	timeout := time.Duration(timeoutMilliseconds) * time.Millisecond
	return errorString(controller.WaitReady(timeout))
}

//export tungo_stop
func tungo_stop(controllerHandle C.uintptr_t) *C.char {
	controller, err := resolveController(cgo.Handle(controllerHandle))
	if err != nil {
		return errorString(err)
	}
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

func resolveController(handle cgo.Handle) (controller *control.Controller, err error) {
	if handle == 0 {
		return nil, fmt.Errorf("controller handle is required")
	}
	defer func() {
		if recover() != nil {
			controller = nil
			err = fmt.Errorf("controller handle is invalid")
		}
	}()
	controller, ok := handle.Value().(*control.Controller)
	if !ok {
		return nil, fmt.Errorf("controller handle has an unexpected type")
	}
	return controller, nil
}

func releaseControllerHandle(handle cgo.Handle) {
	controller, err := resolveController(handle)
	if err != nil {
		return
	}
	_ = controller.Stop()
	deleteHandle(handle)
}

func deleteHandle(handle cgo.Handle) {
	defer func() {
		_ = recover()
	}()
	handle.Delete()
}

func main() {}
