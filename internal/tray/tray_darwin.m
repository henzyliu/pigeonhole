// The menu bar icon: a monochrome symbol that macOS tints for light and dark
// menu bars, the unread count beside it (also shown on the Dock icon), and
// a menu to open or quit Pigeonhole.
#import <Cocoa/Cocoa.h>
#include "_cgo_export.h"

@interface PHTray : NSObject
@property (nonatomic, strong) NSStatusItem *item;
@end

@implementation PHTray
- (void)open:(id)sender {
    // Closing the window hides the whole app (like ⌘H), so unhide it first.
    [NSApp unhide:nil];
    [NSApp activateIgnoringOtherApps:YES];
    pigeonholeTrayOpen();
}
- (void)quit:(id)sender {
    pigeonholeTrayQuit();
}
@end

static PHTray *tray;

static NSImage *PHTrayImage(void) {
    NSImage *image = nil;
    if (@available(macOS 11.0, *)) {
        // "bird" arrived in macOS 13; older systems get the inbox tray.
        for (NSString *name in @[@"bird.fill", @"tray.fill"]) {
            image = [NSImage imageWithSystemSymbolName:name accessibilityDescription:@"Pigeonhole"];
            if (image) break;
        }
    }
    image.template = YES;
    return image;
}

void ph_tray_start(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (tray) return;
        tray = [PHTray new];
        tray.item = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
        NSImage *image = PHTrayImage();
        if (image) {
            tray.item.button.image = image;
            tray.item.button.imagePosition = NSImageLeft;
        } else {
            tray.item.button.title = @"Pigeonhole";
        }
        tray.item.button.toolTip = @"Pigeonhole";

        NSMenu *menu = [NSMenu new];
        NSMenuItem *open = [[NSMenuItem alloc] initWithTitle:@"Open Pigeonhole" action:@selector(open:) keyEquivalent:@""];
        open.target = tray;
        [menu addItem:open];
        [menu addItem:[NSMenuItem separatorItem]];
        NSMenuItem *quit = [[NSMenuItem alloc] initWithTitle:@"Quit Pigeonhole" action:@selector(quit:) keyEquivalent:@"q"];
        quit.target = tray;
        [menu addItem:quit];
        tray.item.menu = menu;
    });
}

void ph_tray_set_unread(int n) {
    dispatch_async(dispatch_get_main_queue(), ^{
        NSString *label = n > 0 ? [NSString stringWithFormat:@"%d", n] : nil;
        if (tray.item.button.image) {
            tray.item.button.title = label ? [@" " stringByAppendingString:label] : @"";
        }
        tray.item.button.toolTip = label ? [NSString stringWithFormat:@"Pigeonhole: %@ unread", label] : @"Pigeonhole";
        NSApp.dockTile.badgeLabel = label;
    });
}

void ph_tray_hide_app(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        [NSApp hide:nil];
    });
}
