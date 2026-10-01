//go:build darwin && !ios && !server

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

static WKWebView *findWebView(NSView *view) {
    if ([view isKindOfClass:[WKWebView class]]) return (WKWebView *)view;
    for (NSView *child in view.subviews) {
        WKWebView *found = findWebView(child);
        if (found) return found;
    }
    return nil;
}

void gulAcceptanceProbe(void *pointer, const char *source) {
    NSWindow *window = (NSWindow *)pointer;
    NSString *script = [NSString stringWithUTF8String:source];
    __block NSUInteger attempts = 0;
    __block NSTimer *timer;
    timer = [NSTimer scheduledTimerWithTimeInterval:0.5 repeats:YES block:^(NSTimer *tick) {
        WKWebView *view = findWebView(window.contentView);
        if (++attempts > 80) {
            fprintf(stdout, "GUL_NATIVE_ACCEPTANCE:timeout\n"); fflush(stdout);
            [tick invalidate]; return;
        }
        [view evaluateJavaScript:script completionHandler:^(id result, NSError *error) {
            if ([result isKindOfClass:[NSString class]] && ![result isEqualToString:@"waiting"]) {
                fprintf(stdout, "GUL_NATIVE_ACCEPTANCE:%s\n", [(NSString *)result UTF8String]); fflush(stdout);
                [timer invalidate];
            }
        }];
    }];
}
