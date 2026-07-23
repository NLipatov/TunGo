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

	"tungo/infrastructure/PAL/network/darwin/ne"
)

var controller = ne.NewController()

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
	*output = C.CString(string(networkSettings))
	return nil
}

//export tungo_start
func tungo_start(tunnelFileDescriptor C.int32_t) *C.char {
	if err := controller.Start(int(tunnelFileDescriptor)); err != nil {
		return errorString(err)
	}
	return nil
}

//export tungo_wait_ready
func tungo_wait_ready(timeoutMilliseconds C.int64_t) *C.char {
	if timeoutMilliseconds <= 0 || timeoutMilliseconds > 300000 {
		return errorString(fmt.Errorf("startup timeout %dms is outside the supported range", int64(timeoutMilliseconds)))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMilliseconds)*time.Millisecond)
	defer cancel()
	if err := controller.WaitReady(ctx); err != nil {
		return errorString(err)
	}
	return nil
}

//export tungo_stop
func tungo_stop() *C.char {
	if err := controller.Stop(); err != nil {
		return errorString(err)
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
