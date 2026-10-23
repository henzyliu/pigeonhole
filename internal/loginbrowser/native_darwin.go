//go:build darwin

package loginbrowser

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#include <stdlib.h>

void ph_login_open(int id, const char *request);
void ph_login_close(int id);
*/
import "C"

import "unsafe"

const supported = true

func nativeOpen(id int, request string) {
	cs := C.CString(request)
	defer C.free(unsafe.Pointer(cs))
	C.ph_login_open(C.int(id), cs)
}

func nativeClose(id int) {
	C.ph_login_close(C.int(id))
}

//export pigeonholeLoginSnapshot
func pigeonholeLoginSnapshot(id C.int, data *C.char) {
	handleSnapshot(int(id), C.GoString(data))
}

//export pigeonholeLoginClosed
func pigeonholeLoginClosed(id C.int) {
	handleClosed(int(id))
}
