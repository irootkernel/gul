//go:build darwin && !ios && !server

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import <Security/Security.h>
#import <CommonCrypto/CommonDigest.h>
#import <objc/runtime.h>
#import "native_https_darwin.h"

@interface GulPinnedDelegate : NSObject <WKNavigationDelegate, WKUIDelegate> {
@public
    uint8_t pin[CC_SHA256_DIGEST_LENGTH];
}
@property (nonatomic, strong) id<WKNavigationDelegate> originalNavigation;
@property (nonatomic, strong) id<WKUIDelegate> originalUI;
@property (nonatomic, strong) NSURL *origin;
@property (nonatomic) BOOL pinnedForNavigation;
@property (nonatomic) int64_t validFrom;
@property (nonatomic) int64_t validUntil;
@end

@interface GulForwardProbe : NSObject <WKNavigationDelegate>
@property (nonatomic) BOOL called;
@end

@implementation GulForwardProbe
- (void)webView:(WKWebView *)webView didFinishNavigation:(WKNavigation *)navigation {
    self.called = YES;
}
@end

@implementation GulPinnedDelegate

- (BOOL)allowsURL:(NSURL *)url {
    if (!url || ![url.scheme isEqualToString:@"https"] ||
        ![url.host isEqualToString:@"127.0.0.1"] || url.user || url.password ||
        !url.port || ![url.port isEqualToNumber:self.origin.port]) {
        return NO;
    }
    return YES;
}

- (BOOL)matchesTrust:(SecTrustRef)trust {
    if (!trust) return NO;
    NSTimeInterval now = [NSDate date].timeIntervalSince1970;
    if (now < self.validFrom || now > self.validUntil) return NO;
    CFArrayRef chain = SecTrustCopyCertificateChain(trust);
    if (!chain || CFArrayGetCount(chain) == 0) {
        if (chain) CFRelease(chain);
        return NO;
    }
    SecCertificateRef leaf = (SecCertificateRef)CFArrayGetValueAtIndex(chain, 0);
    CFDataRef der = SecCertificateCopyData(leaf);
    uint8_t digest[CC_SHA256_DIGEST_LENGTH];
    BOOL matches = der && CC_SHA256(CFDataGetBytePtr(der), (CC_LONG)CFDataGetLength(der), digest) &&
        memcmp(digest, pin, sizeof(digest)) == 0;
    if (der) CFRelease(der);
    CFRelease(chain);
    return matches;
}

- (BOOL)respondsToSelector:(SEL)selector {
    return [super respondsToSelector:selector] ||
        [self.originalNavigation respondsToSelector:selector] ||
        [self.originalUI respondsToSelector:selector];
}

- (id)forwardingTargetForSelector:(SEL)selector {
    if ([self.originalNavigation respondsToSelector:selector]) return self.originalNavigation;
    if ([self.originalUI respondsToSelector:selector]) return self.originalUI;
    return [super forwardingTargetForSelector:selector];
}

- (void)webView:(WKWebView *)webView decidePolicyForNavigationAction:(WKNavigationAction *)action
    decisionHandler:(void (^)(WKNavigationActionPolicy))decisionHandler {
    if (!action.targetFrame || ![self allowsURL:action.request.URL]) {
        decisionHandler(WKNavigationActionPolicyCancel);
        return;
    }
    if ([self.originalNavigation respondsToSelector:_cmd]) {
        [self.originalNavigation webView:webView decidePolicyForNavigationAction:action decisionHandler:decisionHandler];
    } else {
        decisionHandler(WKNavigationActionPolicyAllow);
    }
}

- (void)webView:(WKWebView *)webView decidePolicyForNavigationResponse:(WKNavigationResponse *)response
    decisionHandler:(void (^)(WKNavigationResponsePolicy))decisionHandler {
    BOOL originAllowed = [self allowsURL:response.response.URL];
    BOOL trustMatched = self.pinnedForNavigation || [self matchesTrust:webView.serverTrust];
    self.pinnedForNavigation = NO;
    if (!originAllowed || !trustMatched) {
        decisionHandler(WKNavigationResponsePolicyCancel);
        return;
    }
    if ([self.originalNavigation respondsToSelector:_cmd]) {
        [self.originalNavigation webView:webView decidePolicyForNavigationResponse:response decisionHandler:decisionHandler];
    } else {
        decisionHandler(WKNavigationResponsePolicyAllow);
    }
}

- (void)webView:(WKWebView *)webView didCommitNavigation:(WKNavigation *)navigation {
    if (![self matchesTrust:webView.serverTrust]) {
        [webView stopLoading];
        // Challenge and response policy deny unpinned documents before commit.
        // Stop here without initiating a blank load our URL policy would deny.
        return;
    }
    if ([self.originalNavigation respondsToSelector:_cmd]) {
        [self.originalNavigation webView:webView didCommitNavigation:navigation];
    }
}

- (void)webView:(WKWebView *)webView didReceiveAuthenticationChallenge:(NSURLAuthenticationChallenge *)challenge
    completionHandler:(void (^)(NSURLSessionAuthChallengeDisposition, NSURLCredential *))completionHandler {
    NSURLProtectionSpace *space = challenge.protectionSpace;
    if (![space.authenticationMethod isEqualToString:NSURLAuthenticationMethodServerTrust] ||
        ![space.protocol isEqualToString:@"https"] ||
        ![space.host isEqualToString:@"127.0.0.1"] ||
        space.port != self.origin.port.integerValue || !space.serverTrust) {
        completionHandler(NSURLSessionAuthChallengeCancelAuthenticationChallenge, nil);
        return;
    }
    BOOL challengeMatched = [self matchesTrust:space.serverTrust];
    if (!challengeMatched) {
        completionHandler(NSURLSessionAuthChallengeCancelAuthenticationChallenge, nil);
        return;
    }
    self.pinnedForNavigation = YES;
    completionHandler(NSURLSessionAuthChallengeUseCredential,
        [NSURLCredential credentialForTrust:space.serverTrust]);
}

- (WKWebView *)webView:(WKWebView *)webView createWebViewWithConfiguration:(WKWebViewConfiguration *)configuration
    forNavigationAction:(WKNavigationAction *)navigationAction windowFeatures:(WKWindowFeatures *)windowFeatures {
    return nil;
}
@end

static const char gulDelegateKey;

// NSApplication terminate exits the process without returning through Go.
// Stop the event loop instead so the caller reports errors and drains its core.
void gulStopNativeApplication(void) {
    [NSApp stop:nil];
    NSEvent *wake = [NSEvent otherEventWithType:NSEventTypeApplicationDefined
        location:NSZeroPoint modifierFlags:0 timestamp:0 windowNumber:0
        context:nil subtype:0 data1:0 data2:0];
    [NSApp postEvent:wake atStart:YES];
}

bool gulInstallNativeHTTPS(void *windowPointer, const char *originString, const uint8_t *pin, const char *scriptString,
                           int64_t validFrom, int64_t validUntil) {
    if (![NSThread isMainThread] || !windowPointer || !originString || !pin || !scriptString) return false;
    NSWindow *window = (__bridge NSWindow *)windowPointer;
    WKWebView *webView = nil;
    for (NSView *view in window.contentView.subviews) {
        if ([view isKindOfClass:[WKWebView class]]) {
            webView = (WKWebView *)view;
            break;
        }
    }
    if (!webView || objc_getAssociatedObject(webView, &gulDelegateKey)) return false;
    NSURL *origin = [NSURL URLWithString:[NSString stringWithUTF8String:originString]];
    NSString *script = [NSString stringWithUTF8String:scriptString];
    if (!origin || !script || ![origin.scheme isEqualToString:@"https"] ||
        ![origin.host isEqualToString:@"127.0.0.1"] || !origin.port) return false;
    GulPinnedDelegate *delegate = [GulPinnedDelegate new];
    delegate.originalNavigation = webView.navigationDelegate;
    delegate.originalUI = webView.UIDelegate;
    delegate.origin = origin;
    delegate.validFrom = validFrom;
    delegate.validUntil = validUntil;
    memcpy(delegate->pin, pin, CC_SHA256_DIGEST_LENGTH);
    if (script.length > 0) {
        WKUserScript *bootstrap = [[WKUserScript alloc] initWithSource:script
            injectionTime:WKUserScriptInjectionTimeAtDocumentStart forMainFrameOnly:YES];
        [webView.configuration.userContentController addUserScript:bootstrap];
    }
    objc_setAssociatedObject(webView, &gulDelegateKey, delegate, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
    webView.navigationDelegate = delegate;
    webView.UIDelegate = delegate;
    return true;
}

bool gulProbeNativeHTTPS(const char *originString, const char *candidateString, const uint8_t *pin,
                         const uint8_t *derBytes, int derLength, int64_t validFrom, int64_t validUntil) {
    if (!originString || !candidateString || !pin || !derBytes || derLength <= 0) return false;
    GulPinnedDelegate *delegate = [GulPinnedDelegate new];
    delegate.origin = [NSURL URLWithString:[NSString stringWithUTF8String:originString]];
    delegate.validFrom = validFrom;
    delegate.validUntil = validUntil;
    memcpy(delegate->pin, pin, CC_SHA256_DIGEST_LENGTH);
    if (![delegate allowsURL:[NSURL URLWithString:[NSString stringWithUTF8String:candidateString]]]) return false;
    CFDataRef data = CFDataCreate(kCFAllocatorDefault, derBytes, derLength);
    SecCertificateRef certificate = SecCertificateCreateWithData(kCFAllocatorDefault, data);
    CFRelease(data);
    if (!certificate) return false;
    SecTrustRef trust = nil;
    SecPolicyRef policy = SecPolicyCreateBasicX509();
    OSStatus status = SecTrustCreateWithCertificates(certificate, policy, &trust);
    CFRelease(policy);
    CFRelease(certificate);
    BOOL matches = status == errSecSuccess && [delegate matchesTrust:trust];
    if (trust) CFRelease(trust);
    return matches;
}

bool gulProbeDelegateForwarding(void) {
    GulForwardProbe *original = [GulForwardProbe new];
    GulPinnedDelegate *wrapper = [GulPinnedDelegate new];
    wrapper.originalNavigation = original;
    SEL selector = @selector(webView:didFinishNavigation:);
    if (![wrapper respondsToSelector:selector]) return false;
    WKWebView *webView = (WKWebView *)[NSObject new];
    [(id<WKNavigationDelegate>)wrapper webView:webView didFinishNavigation:nil];
    return original.called;
}
