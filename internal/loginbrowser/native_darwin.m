// Native login window for macOS: an NSWindow with a WKWebView that reports
// snapshots (URL, cookies, localStorage, ExtractJS results) back to Go. Go
// decides when the login is complete and closes the window.

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#include "_cgo_export.h"

@interface PHLoginController : NSObject <WKNavigationDelegate, NSWindowDelegate>
@property (nonatomic) int requestID;
@property (nonatomic, strong) NSWindow *window;
@property (nonatomic, strong) WKWebView *webView;
@property (nonatomic, strong) NSArray<NSString *> *localStorageKeys;
@property (nonatomic, copy) NSString *extractJS;
@property (nonatomic, strong) NSDictionary *extracted;
@property (nonatomic) BOOL extractRunning;
@property (nonatomic) BOOL snapshotRunning;
@property (nonatomic, strong) NSTimer *timer;
@property (nonatomic) BOOL closed;
@end

// Controllers are kept alive here until their window closes.
static NSMutableDictionary<NSNumber *, PHLoginController *> *controllers;

static NSString *PHSafariUserAgentSuffix(void) {
    NSBundle *safari = [NSBundle bundleWithPath:@"/Applications/Safari.app"];
    NSString *version = safari.infoDictionary[@"CFBundleShortVersionString"];
    if (![version isKindOfClass:[NSString class]] || version.length == 0) {
        version = @"18.0";
    }
    return [NSString stringWithFormat:@"Version/%@ Safari/605.1.15", version];
}

@implementation PHLoginController

- (void)startWithRequest:(NSDictionary *)req {
    WKWebViewConfiguration *config = [[WKWebViewConfiguration alloc] init];
    // Without this, WKWebView's user agent has no "Version/… Safari/…" suffix,
    // so sites (Instagram included) treat it as an unknown embedded browser.
    // Appending the installed Safari's version makes it identical to Safari.
    config.applicationNameForUserAgent = PHSafariUserAgentSuffix();
    // A private, in-memory store: isolated from the user's own browsers and
    // gone when the window closes.
    config.websiteDataStore = [WKWebsiteDataStore nonPersistentDataStore];

    self.webView = [[WKWebView alloc] initWithFrame:NSMakeRect(0, 0, 520, 760) configuration:config];
    self.webView.navigationDelegate = self;
    NSString *ua = req[@"userAgent"];
    if ([ua isKindOfClass:[NSString class]] && ua.length > 0) {
        self.webView.customUserAgent = ua;
    }
    NSArray *keys = req[@"localStorageKeys"];
    self.localStorageKeys = [keys isKindOfClass:[NSArray class]] ? keys : @[];
    NSString *js = req[@"extractJS"];
    self.extractJS = [js isKindOfClass:[NSString class]] ? js : @"";

    self.window = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 520, 760)
                                              styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable |
                                                        NSWindowStyleMaskResizable | NSWindowStyleMaskMiniaturizable
                                                backing:NSBackingStoreBuffered
                                                  defer:NO];
    self.window.releasedWhenClosed = NO;
    self.window.delegate = self;
    NSString *title = req[@"title"];
    self.window.title = [title isKindOfClass:[NSString class]] ? title : @"Log in";
    self.window.contentView = self.webView;
    [self.window center];
    if (![req[@"hidden"] boolValue]) {
        [self.window makeKeyAndOrderFront:nil];
        [NSApp activateIgnoringOtherApps:YES];
    }

    // Set any cookies the bridge provided (to continue a partial login), then load.
    WKHTTPCookieStore *store = config.websiteDataStore.httpCookieStore;
    dispatch_group_t group = dispatch_group_create();
    NSArray *initial = req[@"initialCookies"];
    if ([initial isKindOfClass:[NSArray class]]) {
        for (NSDictionary *c in initial) {
            NSMutableDictionary *props = [NSMutableDictionary dictionary];
            props[NSHTTPCookieName] = c[@"name"] ?: @"";
            props[NSHTTPCookieValue] = c[@"value"] ?: @"";
            props[NSHTTPCookieDomain] = c[@"domain"] ?: @"";
            NSString *path = c[@"path"];
            props[NSHTTPCookiePath] = ([path isKindOfClass:[NSString class]] && path.length) ? path : @"/";
            if ([c[@"secure"] boolValue]) props[NSHTTPCookieSecure] = @"TRUE";
            NSHTTPCookie *cookie = [NSHTTPCookie cookieWithProperties:props];
            if (!cookie) continue;
            dispatch_group_enter(group);
            [store setCookie:cookie completionHandler:^{ dispatch_group_leave(group); }];
        }
    }
    NSURL *url = [NSURL URLWithString:req[@"url"] ?: @""];
    dispatch_group_notify(group, dispatch_get_main_queue(), ^{
        if (url) [self.webView loadRequest:[NSURLRequest requestWithURL:url]];
    });

    self.timer = [NSTimer scheduledTimerWithTimeInterval:1.0
                                                  target:self
                                                selector:@selector(snapshot)
                                                userInfo:nil
                                                 repeats:YES];
}

- (void)webView:(WKWebView *)webView didFinishNavigation:(WKNavigation *)navigation {
    [self runExtract];
    [self snapshot];
}

// Runs the bridge's ExtractJS, which evaluates to a promise of field values
// (e.g. Instagram's CAPTCHA token once solved). A navigation resets the page,
// so it's re-run after each load.
- (void)runExtract {
    if (self.extractJS.length == 0 || self.extractRunning || self.closed) return;
    if (@available(macOS 11.0, *)) {
        self.extractRunning = YES;
        NSString *body = [NSString stringWithFormat:@"return await (%@);", self.extractJS];
        [self.webView callAsyncJavaScript:body
                                arguments:@{}
                                  inFrame:nil
                           inContentWorld:WKContentWorld.pageWorld
                        completionHandler:^(id result, NSError *error) {
            self.extractRunning = NO;
            if ([result isKindOfClass:[NSDictionary class]]) {
                self.extracted = result;
                [self snapshot];
            } else if (error && !self.closed) {
                // A page load interrupted it (and its own re-run was skipped
                // while this one was still running), so start it again.
                dispatch_after(dispatch_time(DISPATCH_TIME_NOW, NSEC_PER_SEC), dispatch_get_main_queue(), ^{
                    [self runExtract];
                });
            }
        }];
    }
}

- (void)snapshot {
    if (self.closed || self.snapshotRunning) return;
    self.snapshotRunning = YES;
    NSString *url = self.webView.URL.absoluteString ?: @"";
    [self.webView.configuration.websiteDataStore.httpCookieStore getAllCookies:^(NSArray<NSHTTPCookie *> *cookies) {
        NSMutableArray *list = [NSMutableArray array];
        for (NSHTTPCookie *c in cookies) {
            [list addObject:@{@"name": c.name ?: @"", @"value": c.value ?: @"", @"domain": c.domain ?: @""}];
        }
        [self readLocalStorage:^(NSDictionary *storage) {
            self.snapshotRunning = NO;
            if (self.closed) return;
            NSDictionary *snap = @{
                @"url": url,
                @"cookies": list,
                @"localStorage": storage ?: @{},
                @"extracted": self.extracted ?: @{},
            };
            NSData *data = [NSJSONSerialization dataWithJSONObject:snap options:0 error:nil];
            if (!data) return;
            NSString *json = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
            pigeonholeLoginSnapshot(self.requestID, (char *)json.UTF8String);
        }];
    }];
}

- (void)readLocalStorage:(void (^)(NSDictionary *))done {
    if (self.localStorageKeys.count == 0) {
        done(@{});
        return;
    }
    NSData *keys = [NSJSONSerialization dataWithJSONObject:self.localStorageKeys options:0 error:nil];
    NSString *keysJSON = [[NSString alloc] initWithData:keys encoding:NSUTF8StringEncoding];
    NSString *script = [NSString stringWithFormat:
        @"(() => { const o = {}; try { for (const k of %@) { const v = localStorage.getItem(k); if (v !== null) o[k] = v; } } catch (e) {} return o; })()",
        keysJSON];
    [self.webView evaluateJavaScript:script completionHandler:^(id result, NSError *error) {
        done([result isKindOfClass:[NSDictionary class]] ? result : @{});
    }];
}

- (void)stop {
    self.closed = YES;
    [self.timer invalidate];
    self.timer = nil;
    [self.webView stopLoading];
    [controllers removeObjectForKey:@(self.requestID)];
}

// Closed by Go once the login is complete (or cancelled).
- (void)closeFromGo {
    if (self.closed) return;
    [self stop];
    [self.window close];
}

// Closed by the user.
- (void)windowWillClose:(NSNotification *)notification {
    if (self.closed) return;
    int requestID = self.requestID;
    [self stop];
    pigeonholeLoginClosed(requestID);
}

@end

void ph_login_open(int requestID, const char *request) {
    NSData *data = [NSData dataWithBytes:request length:strlen(request)];
    dispatch_async(dispatch_get_main_queue(), ^{
        NSDictionary *req = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
        if (![req isKindOfClass:[NSDictionary class]]) {
            pigeonholeLoginClosed(requestID);
            return;
        }
        if (!controllers) controllers = [NSMutableDictionary dictionary];
        PHLoginController *controller = [[PHLoginController alloc] init];
        controller.requestID = requestID;
        controllers[@(requestID)] = controller;
        [controller startWithRequest:req];
    });
}

void ph_login_close(int requestID) {
    dispatch_async(dispatch_get_main_queue(), ^{
        [controllers[@(requestID)] closeFromGo];
    });
}
