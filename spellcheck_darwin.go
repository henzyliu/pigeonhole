package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation
#import <Foundation/Foundation.h>

// WebKit only underlines misspellings in apps that have opted in (Safari
// does, through its Spelling menu); it reads this once, when the first web
// view is made.
static void enableSpellChecking(void) {
	[[NSUserDefaults standardUserDefaults] setBool:YES forKey:@"WebContinuousSpellCheckingEnabled"];
}
*/
import "C"

// enableSpellChecking turns on macOS's spell checker in the web view. The
// Spell check setting then decides, per text box, whether it's used.
func enableSpellChecking() { C.enableSpellChecking() }
