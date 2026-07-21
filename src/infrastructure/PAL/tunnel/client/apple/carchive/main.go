//go:build darwin || ios

package main

/*
#include <stdint.h>
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"fmt"
	"time"
	"unsafe"

	appleClient "tungo/infrastructure/PAL/tunnel/client/apple"
)

var controller = appleClient.NewController()

//export tungo_plan
func tungo_plan(output **C.char) *C.char {
	if output == nil {
		return errorString(fmt.Errorf("output plan pointer is required"))
	}
	plan, err := controller.TunnelPlan()
	if err != nil {
		return errorString(err)
	}
	*output = C.CString(string(plan))
	return nil
}

//export tungo_start
func tungo_start(
	tunnelFileDescriptor C.int32_t,
	outputHandle *C.uint64_t,
) *C.char {
	if outputHandle == nil {
		return errorString(fmt.Errorf("output tunnel handle is required"))
	}
	handle, err := controller.Start(int(tunnelFileDescriptor))
	if err != nil {
		return errorString(err)
	}
	*outputHandle = C.uint64_t(handle)
	return nil
}

//export tungo_wait_ready
func tungo_wait_ready(handle C.uint64_t, timeoutMilliseconds C.int64_t) *C.char {
	if timeoutMilliseconds <= 0 || timeoutMilliseconds > 300000 {
		return errorString(fmt.Errorf("startup timeout %dms is outside the supported range", int64(timeoutMilliseconds)))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMilliseconds)*time.Millisecond)
	defer cancel()
	if err := controller.WaitReady(ctx, uint64(handle)); err != nil {
		return errorString(err)
	}
	return nil
}

//export tungo_stop
func tungo_stop(handle C.uint64_t) *C.char {
	if err := controller.Stop(uint64(handle)); err != nil {
		return errorString(err)
	}
	return nil
}

//export tungo_pause
func tungo_pause(handle C.uint64_t) *C.char {
	if err := controller.Pause(uint64(handle)); err != nil {
		return errorString(err)
	}
	return nil
}

//export tungo_restart
func tungo_restart(handle C.uint64_t) *C.char {
	if err := controller.Restart(uint64(handle)); err != nil {
		return errorString(err)
	}
	return nil
}

//export tungo_status
func tungo_status(handle C.uint64_t, outputState *C.int32_t, outputDetail **C.char) *C.char {
	if outputState == nil {
		return errorString(fmt.Errorf("output state pointer is required"))
	}
	status, err := controller.Status(uint64(handle))
	if err != nil {
		return errorString(err)
	}
	*outputState = C.int32_t(status.State)
	if outputDetail != nil {
		if status.Error == "" {
			*outputDetail = nil
		} else {
			*outputDetail = C.CString(status.Error)
		}
	}
	return nil
}

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
