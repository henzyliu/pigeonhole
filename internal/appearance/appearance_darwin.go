//go:build darwin

package appearance

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// name is "light", "dark", or anything else to follow the system.
static void ph_set_appearance(const char *name) {
    NSString *theme = [NSString stringWithUTF8String:name];
    dispatch_async(dispatch_get_main_queue(), ^{
        NSAppearance *appearance = nil;
        if ([theme isEqualToString:@"dark"]) {
            appearance = [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua];
        } else if ([theme isEqualToString:@"light"]) {
            appearance = [NSAppearance appearanceNamed:NSAppearanceNameAqua];
        }
        // nil means "follow the system". Setting it on NSApp covers every
        // window, including the login browser.
        NSApp.appearance = appearance;
    });
}
*/
import "C"

import "unsafe"

// Apply sets the whole app's native appearance. Wails' WindowSet*Theme
// functions are no-ops on macOS, hence the direct call.
func Apply(theme string) {
	cs := C.CString(theme)
	defer C.free(unsafe.Pointer(cs))
	C.ph_set_appearance(cs)
}
