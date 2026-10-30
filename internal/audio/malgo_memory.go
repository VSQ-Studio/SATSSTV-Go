//go:build cgo

package audio

/*
#include <stdlib.h>
*/
import "C"

import "unsafe"

func freeMalgoDeviceID(ptr unsafe.Pointer) {
	if ptr != nil {
		C.free(ptr)
	}
}
