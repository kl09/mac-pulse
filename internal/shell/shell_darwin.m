#import <Carbon/Carbon.h>
#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

#include <stdatomic.h>

#include "_cgo_export.h"
#include "shell.h"

static const CGFloat kPanelWidth = 420, kPanelHeight = 704, kPanelRadius = 18, kEdgeGap = 8;
// Points from the right screen edge; anything below Control Center's own offset lands just left of it.
// Items with no place of their own yet count down from the first, so each lands right of the one
// made before it, and the clock right of them all.
static const double kStatusFirstPosition = 100, kStatusClockPosition = 50;
// AppKit's key for where an item sits, and ours for where a removed one sat; the autosave name follows.
static NSString *const kPositionKey = @"NSStatusItem Preferred Position ";
static NSString *const kKeptPositionKey = @"mac-pulse kept position ";
// Where the user left the panel, as NSStringFromRect; absent until it is first dragged or resized.
static NSString *const kPanelFrameKey = @"mac-pulse panel frame";
// How much of the panel's top edge must be on a screen for a saved place to be used: enough to grab and drag it back.
static const CGFloat kGripWidth = 80, kGripHeight = 40;
static const CGFloat kWindowWidth = 1000, kWindowHeight = 660, kWindowMinWidth = 900, kWindowMinHeight = 600;
static const NSTimeInterval kFadeIn = 0.12;
// The frontend's messages are a few hundred bytes; anything near this is not ours.
static const NSUInteger kMaxMessageLength = 64 * 1024;
// Long enough for the sampler's push after the panel opens to land before the picture is taken.
static const NSTimeInterval kMenuExportDelay = 1.5;
static const int64_t kExportTimeout = 5 * NSEC_PER_SEC;
// How long a Go caller waits for the main queue: after mpQuit nothing drains it any more.
static const int64_t kMainTimeout = 2 * NSEC_PER_SEC;
static const int64_t kChooseFolderPoll = NSEC_PER_SEC / 5;
static const CGFloat kSparkWidth = 24, kSparkInset = 2;

// Main thread only, like menuTitles: Go may set them before the shell object exists.
static BOOL debugMode, compactStatus, windowOnTop, dockSetting;
static void applyDock(void);
// Main thread only: nil follows the system, else what the panel and the window are forced to.
static NSAppearance *forcedAppearance;
// Main thread only. English until Go names the items in the interface language.
static NSArray<NSString *> *menuTitles;
// Main thread only: the choose-a-folder dialog while it is up.
static NSOpenPanel *folderPanel;
// Main thread only: how many items statusItemNamed: has placed by itself.
static int placedItems;
// Set by mpQuit on any thread; mpChooseFolder stops waiting once it is.
static atomic_bool quitting;

@interface MPShell : NSObject <NSApplicationDelegate,
                               NSWindowDelegate,
                               NSUserNotificationCenterDelegate,
                               WKScriptMessageHandler,
                               WKURLSchemeHandler,
                               WKNavigationDelegate>
- (void)closePanel;
- (void)togglePanel;
@end

@interface MPWebView : WKWebView
@end

@implementation MPWebView
// Reload, Back and Look Up make no sense in a panel; the menu survives in debug mode for Inspect Element.
- (void)willOpenMenu:(NSMenu *)menu withEvent:(NSEvent *)event {
    if (!debugMode) {
        [menu removeAllItems];
    }
}

// Neither the panel nor the window has a title bar of its own (the page fills both), so a press
// on the page's grabber, its title strip or between the header's controls drags it; a press on a control or inside the content stays the page's own. The page answers from
// its DOM, after the press has reached it, and can mark more of itself with data-drag.
- (void)mouseDown:(NSEvent *)event {
    [super mouseDown:event];
    NSPoint at = [self convertPoint:event.locationInWindow fromView:nil];
    NSString *js = [NSString stringWithFormat:@"(function(e){return !e||!e.closest('button,a,input,select,textarea,label,summary,[role],[tabindex],"
                                              @"[contenteditable]')&&(!e.closest('main')||!!e.closest('[data-drag]'))})"
                                              @"(document.elementFromPoint(%f,%f))",
                                              at.x, self.isFlipped ? at.y : NSHeight(self.bounds) - at.y];
    [self evaluateJavaScript:js
           completionHandler:^(id drag, NSError *err) {
               // The answer takes a few milliseconds; a click that has already ended is not a drag.
               if ([drag isKindOfClass:NSNumber.class] && [drag boolValue] && (NSEvent.pressedMouseButtons & 1)) {
                   [self.window performWindowDragWithEvent:event];
               }
           }];
}
@end

// A borderless panel cannot become key by default; without key status the web view gets no keyboard input.
@interface MPPanel : NSPanel
@end

@implementation MPPanel
- (BOOL)canBecomeKeyWindow {
    return YES;
}

- (void)cancelOperation:(id)sender {
    [(MPShell *)self.delegate closePanel];
}
@end

@interface MPShell ()
@property(copy) NSString *tab, *version;
@property BOOL openAtStart, windowAtStart;
// A pinned panel ignores outside clicks; cleared when the panel closes, which a status item click does.
@property BOOL pinned;
@property NSArray<NSDictionary *> *statusItems;
@property CFAbsoluteTime closedAt;
@property NSStatusItem *item;
// The status items of their own, the tab a click on each opens and what each shows, all by id.
@property NSMutableDictionary<NSString *, NSStatusItem *> *extras;
@property NSMutableDictionary<NSString *, NSString *> *extraTabs;
@property NSMutableDictionary<NSString *, NSArray<NSDictionary *> *> *extraItems;
// nil while there is no clock item.
@property NSDateFormatter *clockFormat;
@property NSTimeInterval clockPeriod;
@property NSTimer *clockTimer;
// The item the panel last opened under; anchorItem replaces it once it is gone or hidden.
@property NSStatusItem *anchor;
@property MPPanel *panel;
// The frame showPanelOnTab: gave the panel, zero while it is giving one; any other frame is the
// user's doing and is remembered.
@property NSRect placed;
@property WKWebView *panelWeb;
@property NSWindow *window;
@property WKWebView *windowWeb;
// Scheme tasks whose body is still being read off the main thread; WebKit raises if a stopped task is answered.
@property NSMutableSet<id<WKURLSchemeTask>> *tasks;
@end

static MPShell *shell;

static void onMain(dispatch_block_t block) {
    dispatch_async(dispatch_get_main_queue(), block);
}

static BOOL isDark(NSView *view) {
    NSAppearanceName name = [view.effectiveAppearance bestMatchFromAppearancesWithNames:@[NSAppearanceNameAqua, NSAppearanceNameDarkAqua]];
    return [name isEqualToString:NSAppearanceNameDarkAqua];
}

static NSString *jsonString(id object) {
    NSData *data = [NSJSONSerialization dataWithJSONObject:object options:NSJSONWritingFragmentsAllowed error:nil];
    return [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
}

// Draws image at 2x over an opaque fill and writes a PNG: a transparent snapshot is unreadable in an image viewer.
static NSError *writePNG(NSImage *image, NSColor *fill, NSString *path, NSDataWritingOptions options) {
    NSSize size = image.size;
    NSBitmapImageRep *rep = [[NSBitmapImageRep alloc] initWithBitmapDataPlanes:NULL
                                                                    pixelsWide:(NSInteger)(size.width * 2)
                                                                    pixelsHigh:(NSInteger)(size.height * 2)
                                                                 bitsPerSample:8
                                                               samplesPerPixel:4
                                                                      hasAlpha:YES
                                                                      isPlanar:NO
                                                                colorSpaceName:NSCalibratedRGBColorSpace
                                                                   bytesPerRow:0
                                                                  bitsPerPixel:0];
    rep.size = size;
    [NSGraphicsContext saveGraphicsState];
    NSGraphicsContext.currentContext = [NSGraphicsContext graphicsContextWithBitmapImageRep:rep];
    NSRect rect = NSMakeRect(0, 0, size.width, size.height);
    [fill setFill];
    NSRectFill(rect);
    [image drawInRect:rect fromRect:NSZeroRect operation:NSCompositingOperationSourceOver fraction:1];
    [NSGraphicsContext restoreGraphicsState];
    NSError *err;
    if ([[rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}] writeToFile:path options:options error:&err]) {
        return nil;
    }
    return err ?: [NSError errorWithDomain:NSCocoaErrorDomain code:NSFileWriteUnknownError userInfo:nil];
}

@implementation MPShell

// A URL that launches the app is delivered before applicationDidFinishLaunching.
- (void)applicationWillFinishLaunching:(NSNotification *)note {
    [NSAppleEventManager.sharedAppleEventManager setEventHandler:self
                                                     andSelector:@selector(handleURLEvent:withReplyEvent:)
                                                   forEventClass:kInternetEventClass
                                                      andEventID:kAEGetURL];
}

- (void)applicationDidFinishLaunching:(NSNotification *)note {
    self.tasks = [NSMutableSet new];
    [self buildMainMenu];
    // Logout and shutdown arrive as a quit Apple event. AppKit's handler would route it through
    // applicationShouldTerminate, whose NSTerminateCancel makes loginwindow abort the logout.
    [NSAppleEventManager.sharedAppleEventManager setEventHandler:self
                                                     andSelector:@selector(handleQuitEvent:withReplyEvent:)
                                                   forEventClass:kCoreEventClass
                                                      andEventID:kAEQuitApplication];
    // nil outside a bundle.
    NSUserNotificationCenter.defaultUserNotificationCenter.delegate = self;
    self.extras = [NSMutableDictionary new];
    self.extraTabs = [NSMutableDictionary new];
    self.extraItems = [NSMutableDictionary new];
    [self setStatus:@[@{@"symbol": @"cpu", @"text": @""}]];

    self.panel = [[MPPanel alloc] initWithContentRect:NSMakeRect(0, 0, kPanelWidth, kPanelHeight)
                                            styleMask:NSWindowStyleMaskBorderless | NSWindowStyleMaskNonactivatingPanel | NSWindowStyleMaskResizable
                                              backing:NSBackingStoreBuffered
                                                defer:NO];
    self.panel.level = NSPopUpMenuWindowLevel;
    // NSPanel hides itself whenever the app is inactive, and a menu bar app is never active.
    self.panel.hidesOnDeactivate = NO;
    self.panel.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorFullScreenAuxiliary;
    self.panel.opaque = NO;
    self.panel.backgroundColor = NSColor.clearColor;
    self.panel.hasShadow = YES;
    self.panel.delegate = self;
    self.panel.appearance = forcedAppearance;

    [NSEvent addGlobalMonitorForEventsMatchingMask:NSEventMaskLeftMouseDown | NSEventMaskRightMouseDown
                                           handler:^(NSEvent *event) {
                                               if (self.panel.isVisible && !self.pinned) {
                                                   [self closePanel];
                                               }
                                           }];
    // The status bar positions the new item on a later run loop turn; the panel anchors to it.
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW, NSEC_PER_SEC / 2), dispatch_get_main_queue(), ^{
        if (self.openAtStart) {
            self.pinned = YES;
            [self showPanel];
            [self debugPlacePanel];
        }
        if (self.windowAtStart) {
            [self openWindow];
        }
    });
}

// Key equivalents only exist through a main menu: without Edit the web views get no Cmd+C/V/X/A.
- (void)buildMainMenu {
    NSMenu *bar = [NSMenu new];
    NSMenu *app = [NSMenu new];
    [app addItemWithTitle:@"Quit mac-pulse" action:@selector(terminate:) keyEquivalent:@"q"];
    [bar addItemWithTitle:@"" action:nil keyEquivalent:@""].submenu = app;
    NSMenu *edit = [[NSMenu alloc] initWithTitle:@"Edit"];
    [edit addItemWithTitle:@"Cut" action:@selector(cut:) keyEquivalent:@"x"];
    [edit addItemWithTitle:@"Copy" action:@selector(copy:) keyEquivalent:@"c"];
    [edit addItemWithTitle:@"Paste" action:@selector(paste:) keyEquivalent:@"v"];
    [edit addItemWithTitle:@"Select All" action:@selector(selectAll:) keyEquivalent:@"a"];
    [bar addItemWithTitle:@"" action:nil keyEquivalent:@""].submenu = edit;
    NSMenu *window = [[NSMenu alloc] initWithTitle:@"Window"];
    [window addItemWithTitle:@"Close" action:@selector(performClose:) keyEquivalent:@"w"];
    [bar addItemWithTitle:@"" action:nil keyEquivalent:@""].submenu = window;
    NSApp.mainMenu = bar;
}

// Cmd+Q, logout and shutdown go through Go so it can flush state before the run loop stops.
- (NSApplicationTerminateReply)applicationShouldTerminate:(NSApplication *)sender {
    mpMessage("{\"type\":\"quit\"}");
    return NSTerminateCancel;
}

- (void)handleQuitEvent:(NSAppleEventDescriptor *)event withReplyEvent:(NSAppleEventDescriptor *)reply {
    mpMessage("{\"type\":\"quit\"}");
}

// Go decides what the URL means; anyone can send one.
- (void)handleURLEvent:(NSAppleEventDescriptor *)event withReplyEvent:(NSAppleEventDescriptor *)reply {
    NSString *url = [event paramDescriptorForKeyword:keyDirectObject].stringValue;
    if (url.length == 0 || url.length > kMaxMessageLength) {
        return;
    }
    mpMessage((char *)jsonString(@{@"type": @"url", @"url": url}).UTF8String);
}

// Launching the bundle again (Finder, Spotlight, open) lands here instead of starting a second
// process, and so does a click on the Dock icon, which opens the window: the panel belongs to
// the menu bar.
- (BOOL)applicationShouldHandleReopen:(NSApplication *)sender hasVisibleWindows:(BOOL)visible {
    if (NSApp.activationPolicy == NSApplicationActivationPolicyRegular) {
        mpMessage("{\"type\":\"open_window\"}");
    } else {
        [self showPanel];
    }
    return NO;
}

// Without this a banner is suppressed while the detached window is frontmost.
- (BOOL)userNotificationCenter:(NSUserNotificationCenter *)center shouldPresentNotification:(NSUserNotification *)notification {
    return YES;
}

- (void)userNotificationCenter:(NSUserNotificationCenter *)center didActivateNotification:(NSUserNotification *)notification {
    [self showPanelOnTab:notification.userInfo[@"tab"]];
}

// tab is already URL-escaped.
- (WKWebView *)webViewWithMode:(NSString *)mode tab:(NSString *)tab {
    WKWebViewConfiguration *config = [WKWebViewConfiguration new];
    // Nothing worth caching: a persistent store would write ~/Library/WebKit and ~/Library/Caches.
    config.websiteDataStore = WKWebsiteDataStore.nonPersistentDataStore;
    [config setURLSchemeHandler:self forURLScheme:@"mp"];
    [config.userContentController addScriptMessageHandler:self name:@"mp"];
    // The main frame rubber-bands even when nothing scrolls, which gives the web view away.
    NSString *noBounce = @"document.documentElement.style.overscrollBehavior = 'none'";
    [config.userContentController addUserScript:[[WKUserScript alloc] initWithSource:noBounce
                                                                       injectionTime:WKUserScriptInjectionTimeAtDocumentStart
                                                                    forMainFrameOnly:YES]];
    // Without it Tab stops only at text fields, and the quit and chevron buttons are out of reach.
    config.preferences.tabFocusesLinks = YES;
    if (debugMode) {
        [config.preferences setValue:@YES forKey:@"developerExtrasEnabled"];
    }
    WKWebView *web = [[MPWebView alloc] initWithFrame:NSZeroRect configuration:config];
    [web setValue:@NO forKey:@"drawsBackground"];
    if (@available(macOS 13.3, *)) {
        web.inspectable = debugMode;
    }
    web.navigationDelegate = self;
    NSString *url = [NSString stringWithFormat:@"mp://app/index.html?mode=%@&tab=%@&version=%@", mode, tab, self.version];
    [web loadRequest:[NSURLRequest requestWithURL:[NSURL URLWithString:url]]];
    return web;
}

- (NSVisualEffectView *)materialView:(NSVisualEffectMaterial)material radius:(CGFloat)radius hosting:(WKWebView *)web frame:(NSRect)frame {
    NSVisualEffectView *view = [[NSVisualEffectView alloc] initWithFrame:frame];
    view.material = material;
    view.blendingMode = NSVisualEffectBlendingModeBehindWindow;
    // The panel never activates the app, so the default follows-window state would render the inactive grey.
    view.state = NSVisualEffectStateActive;
    if (radius > 0) {
        // maskImage rounds the material and the window shadow; the layer radius clips the web content.
        NSImage *mask = [NSImage imageWithSize:NSMakeSize(radius * 2 + 1, radius * 2 + 1)
                                       flipped:NO
                                drawingHandler:^BOOL(NSRect rect) {
                                    [NSColor.blackColor setFill];
                                    [[NSBezierPath bezierPathWithRoundedRect:rect xRadius:radius yRadius:radius] fill];
                                    return YES;
                                }];
        mask.capInsets = NSEdgeInsetsMake(radius, radius, radius, radius);
        mask.resizingMode = NSImageResizingModeStretch;
        view.maskImage = mask;
        web.wantsLayer = YES;
        web.layer.cornerRadius = radius;
        web.layer.masksToBounds = YES;
    }
    web.frame = view.bounds;
    web.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
    [view addSubview:web];
    return view;
}

// name is the autosave name: AppKit keeps the item's place in the menu bar under it.
- (NSStatusItem *)statusItemNamed:(NSString *)name {
    // A new status item is placed leftmost, which on a notched display with a full menu bar means
    // hidden under the notch and unreachable. On first launch claim the slot next to Control Center
    // instead; AppKit overwrites this key as soon as the user Cmd-drags the item elsewhere. An item
    // that was taken out of the bar comes back where removeItem: found it.
    NSUserDefaults *defaults = NSUserDefaults.standardUserDefaults;
    NSString *positionKey = [kPositionKey stringByAppendingString:name];
    if (![defaults objectForKey:positionKey]) {
        id kept = [defaults objectForKey:[kKeptPositionKey stringByAppendingString:name]];
        double fresh = [name isEqualToString:@"mac-pulse-clock"] ? kStatusClockPosition : kStatusFirstPosition - placedItems++;
        [defaults setObject:kept ?: @(fresh) forKey:positionKey];
    }
    NSStatusItem *item = [NSStatusBar.systemStatusBar statusItemWithLength:NSVariableStatusItemLength];
    item.autosaveName = name;
    NSStatusBarButton *button = item.button;
    button.target = self;
    button.action = @selector(statusClicked:);
    [button sendActionOn:NSEventMaskLeftMouseUp | NSEventMaskRightMouseUp];
    return item;
}

// Takes the main item (name nil) or an extra one out of the menu bar. AppKit forgets where the user
// dragged an item when it is removed and again when it is released, some time later, so the place
// is kept under a key of our own for statusItemNamed:.
- (void)removeItem:(NSString *)name {
    NSStatusItem *item = name ? self.extras[name] : self.item;
    if (!item) {
        return;
    }
    NSUserDefaults *defaults = NSUserDefaults.standardUserDefaults;
    [defaults setObject:[defaults objectForKey:[kPositionKey stringByAppendingString:item.autosaveName]]
                 forKey:[kKeptPositionKey stringByAppendingString:item.autosaveName]];
    if (self.anchor == item) {
        self.anchor = nil;
    }
    [NSStatusBar.systemStatusBar removeStatusItem:item];
    if (name) {
        [self.extras removeObjectForKey:name];
        [self.extraTabs removeObjectForKey:name];
        [self.extraItems removeObjectForKey:name];
    } else {
        self.item = nil;
    }
}

- (void)setStatus:(NSArray<NSDictionary *> *)items {
    self.statusItems = items;
    // Removed rather than hidden: AppKit remembers a hidden item across launches.
    if (items.count == 0) {
        [self removeItem:nil];
        return;
    }
    if (!self.item) {
        self.item = [self statusItemNamed:@"mac-pulse"];
    }
    [self draw:items on:self.item];
}

// No items removes the item.
- (void)setExtra:(NSString *)name tab:(NSString *)tab items:(NSArray<NSDictionary *> *)items {
    if (items.count == 0) {
        [self removeItem:name];
        return;
    }
    NSStatusItem *item = self.extras[name];
    if (!item) {
        item = [self statusItemNamed:[@"mac-pulse-" stringByAppendingString:name]];
        self.extras[name] = item;
    }
    self.extraTabs[name] = tab;
    self.extraItems[name] = items;
    [self draw:items on:item];
}

// format is an NSDateFormatter template; empty removes the clock.
- (void)setClock:(NSString *)format locale:(NSString *)locale {
    [self.clockTimer invalidate];
    self.clockTimer = nil;
    self.clockFormat = nil;
    if (format.length == 0) {
        [self removeItem:@"clock"];
        return;
    }
    self.clockFormat = [NSDateFormatter new];
    self.clockFormat.locale = locale.length > 0 ? [NSLocale localeWithLocaleIdentifier:locale] : NSLocale.currentLocale;
    // The formatter would otherwise keep the zone it was made in after the Mac moves to another.
    self.clockFormat.timeZone = NSTimeZone.localTimeZone;
    [self.clockFormat setLocalizedDateFormatFromTemplate:format];
    self.clockPeriod = [format containsString:@"s"] ? 1 : 60;
    [self tickClock];
}

// Redraws the clock and sleeps until the next second or minute begins. One-shot timers, because
// a repeating one drifts off the boundary after sleep or a clock change.
- (void)tickClock {
    NSDate *now = NSDate.date;
    [self setExtra:@"clock" tab:@"detail:clock" items:@[@{@"symbol": @"", @"text": [self.clockFormat stringFromDate:now] ?: @""}]];
    NSTimeInterval wait = self.clockPeriod - fmod(now.timeIntervalSince1970, self.clockPeriod);
    self.clockTimer = [NSTimer timerWithTimeInterval:wait target:self selector:@selector(tickClock) userInfo:nil repeats:NO];
    // Common modes: the clock keeps going while a menu is open.
    [NSRunLoop.mainRunLoop addTimer:self.clockTimer forMode:NSRunLoopCommonModes];
}

// The item the panel hangs under: the one last clicked while it is still in the menu bar, else the
// main item, else any extra one when there is no main item.
- (NSStatusItem *)anchorItem {
    if (!self.anchor || (self.anchor != self.item && ![self.extras.allValues containsObject:self.anchor])) {
        self.anchor = self.item ?: self.extras.allValues.firstObject;
    }
    return self.anchor;
}

- (void)draw:(NSArray<NSDictionary *> *)items on:(NSStatusItem *)statusItem {
    // Monospaced digits keep the item from changing width, and shoving its neighbours, on every tick.
    // The menu bar's own size, so the item reads like its neighbours.
    CGFloat size = [NSFont menuBarFontOfSize:0].pointSize - (compactStatus ? 2 : 0);
    NSFont *font = [NSFont monospacedDigitSystemFontOfSize:size weight:NSFontWeightRegular];
    NSImageSymbolConfiguration *symbolConfig = [NSImageSymbolConfiguration configurationWithPointSize:size + 1 weight:NSFontWeightRegular];
    NSMutableAttributedString *title = [NSMutableAttributedString new];
    NSMutableArray<NSString *> *spoken = [NSMutableArray new];
    for (NSDictionary *item in items) {
        if (![item isKindOfClass:NSDictionary.class]) {
            continue;
        }
        NSString *symbol = item[@"symbol"], *text = item[@"text"];
        // Explicit for text and glyph alike: the glyph is drawn by hand, so it cannot inherit the
        // bar's title colour the way the text would.
        NSColor *tint = [item[@"warn"] boolValue] ? NSColor.systemOrangeColor : NSColor.labelColor;
        NSMutableAttributedString *part = [[NSMutableAttributedString alloc] initWithString:title.length == 0 ? @"" : compactStatus ? @"\u2009" : @"\u2009\u2009"];
        NSImage *image = symbol.length > 0 ? [NSImage imageWithSystemSymbolName:symbol accessibilityDescription:nil] : nil;
        if (image) {
            NSImage *mask = [image imageWithSymbolConfiguration:symbolConfig];
            // The handler runs at draw time, where the colour resolves for the menu bar's appearance.
            // The bar's vibrancy draws label-coloured text as if at 0.93 alpha, so the glyph is
            // drawn at that alpha to match. Filling the glyph's own pixels keeps its cut-outs
            // (the "!" in a warning triangle), which a palette colour would paint over.
            NSImage *glyph = [NSImage imageWithSize:mask.size
                                            flipped:NO
                                     drawingHandler:^BOOL(NSRect rect) {
                                         [mask drawInRect:rect];
                                         [[[tint colorUsingColorSpace:NSColorSpace.deviceRGBColorSpace] colorWithAlphaComponent:0.93] set];
                                         NSRectFillUsingOperation(rect, NSCompositingOperationSourceAtop);
                                         return YES;
                                     }];
            NSTextAttachment *attachment = [NSTextAttachment new];
            attachment.image = glyph;
            // An attachment sits on the baseline; centre the glyph on the digits' cap height instead.
            attachment.bounds = NSMakeRect(0, round((font.capHeight - mask.size.height) / 2), mask.size.width, mask.size.height);
            [part appendAttributedString:[NSAttributedString attributedStringWithAttachment:attachment]];
        }
        NSArray *points = item[@"points"];
        if ([points isKindOfClass:NSArray.class] && points.count > 0) {
            NSSize area = NSMakeSize(kSparkWidth + 2 * kSparkInset, ceil(font.capHeight) + 1);
            NSImage *spark = [NSImage imageWithSize:area
                                            flipped:NO
                                     drawingHandler:^BOOL(NSRect rect) {
                                         NSBezierPath *line = [NSBezierPath new];
                                         for (NSUInteger i = 0; i < points.count; i++) {
                                             double value = [points[i] isKindOfClass:NSNumber.class] ? [points[i] doubleValue] : 0;
                                             NSPoint point = NSMakePoint(kSparkInset + kSparkWidth * i / MAX(points.count - 1, 1),
                                                                         0.5 + (rect.size.height - 1) * MAX(0, MIN(1, value)));
                                             if (i == 0) {
                                                 [line moveToPoint:point];
                                             } else {
                                                 [line lineToPoint:point];
                                             }
                                         }
                                         [[[tint colorUsingColorSpace:NSColorSpace.deviceRGBColorSpace] colorWithAlphaComponent:0.93] set];
                                         line.lineJoinStyle = NSLineJoinStyleRound;
                                         [line stroke];
                                         return YES;
                                     }];
            NSTextAttachment *attachment = [NSTextAttachment new];
            attachment.image = spark;
            attachment.bounds = NSMakeRect(0, 0, area.width, area.height);
            [part appendAttributedString:[NSAttributedString attributedStringWithAttachment:attachment]];
        }
        [part.mutableString appendString:text ?: @""];
        [part addAttribute:NSFontAttributeName value:font range:NSMakeRange(0, part.length)];
        [part addAttribute:NSForegroundColorAttributeName value:tint range:NSMakeRange(0, part.length)];
        [title appendAttributedString:part];
        [spoken addObject:text ?: @""];
    }
    statusItem.button.attributedTitle = title;
    statusItem.button.accessibilityTitle = [spoken componentsJoinedByString:@" "];
}

- (void)statusClicked:(id)sender {
    // A click on the main item keeps the panel's tab, as it always did.
    NSStatusItem *clicked = self.item;
    NSString *tab = nil;
    for (NSString *name in self.extras) {
        if (self.extras[name].button == sender) {
            clicked = self.extras[name];
            tab = self.extraTabs[name];
        }
    }
    NSEvent *event = NSApp.currentEvent;
    if (event.type == NSEventTypeRightMouseUp || (event.modifierFlags & NSEventModifierFlagControl)) {
        [self closePanel];
        self.anchor = clicked;
        NSMenu *menu = [NSMenu new];
        NSArray<NSString *> *titles = menuTitles ?: @[@"Open mac-pulse", @"Open in Window", @"Export as Image", @"Settings", @"Exit"];
        NSString *reset = titles.count > 5 ? titles[5] : @"Reset Panel Position";
        [menu addItemWithTitle:titles[0] action:@selector(menuShowPanel:) keyEquivalent:@""].target = self;
        [menu addItemWithTitle:titles[1] action:@selector(menuOpenWindow:) keyEquivalent:@""].target = self;
        [menu addItemWithTitle:titles[2] action:@selector(menuExport:) keyEquivalent:@""].target = self;
        [menu addItemWithTitle:titles[3] action:@selector(menuSettings:) keyEquivalent:@""].target = self;
        [menu addItemWithTitle:reset action:@selector(menuResetPanel:) keyEquivalent:@""].target = self;
        [menu addItem:NSMenuItem.separatorItem];
        [menu addItemWithTitle:titles[4] action:@selector(terminate:) keyEquivalent:@"q"];
        clicked.menu = menu;
        [clicked.button performClick:nil];
        clicked.menu = nil;
        return;
    }
    BOOL same = clicked == [self anchorItem];
    self.pinned = NO;
    if (self.panel.isVisible && same) {
        [self closePanel];
        return;
    }
    // If the mouse-down of this very click dismissed the panel, the mouse-up must not reopen
    // it; a click on another item moves the panel there, unless the user has placed it.
    if (!same || CFAbsoluteTimeGetCurrent() - self.closedAt > 0.25) {
        [self.anchor.button highlight:NO];
        self.anchor = clicked;
        [self showPanelOnTab:tab];
    }
}

- (void)menuShowPanel:(id)sender {
    [self showPanel];
}

- (void)menuSettings:(id)sender {
    [self showPanelOnTab:@"settings"];
}

// Forgets where the user put the panel and shows it under the status item again.
- (void)menuResetPanel:(id)sender {
    [NSUserDefaults.standardUserDefaults removeObjectForKey:kPanelFrameKey];
    [self showPanel];
}

// The MAC_PULSE_PANEL_FRAME debug hook: "x,y,w,h" in screen points from the bottom left puts the open
// panel there as a drag and a resize would, "reset" does what the menu item does.
- (void)debugPlacePanel {
    const char *value = debugMode ? getenv("MAC_PULSE_PANEL_FRAME") : NULL;
    if (!value) {
        return;
    }
    double x, y, width, height;
    if (sscanf(value, "%lf,%lf,%lf,%lf", &x, &y, &width, &height) == 4) {
        [self.panel setFrame:NSMakeRect(x, y, width, height) display:YES];
    } else if (strcmp(value, "reset") == 0) {
        [self menuResetPanel:nil];
    }
}

- (void)menuOpenWindow:(id)sender {
    mpMessage("{\"type\":\"open_window\"}");
}

// Go picks the file name, so the request travels the same road as the frontend's Save image button.
- (void)menuExport:(id)sender {
    if (!self.window.isVisible) {
        [self showPanel];
    }
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(kMenuExportDelay * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
        mpMessage("{\"type\":\"export\"}");
    });
}

- (void)togglePanel {
    self.pinned = NO;
    if (self.panel.isVisible) {
        [self closePanel];
    } else {
        [self showPanel];
    }
}

- (void)showPanel {
    [self showPanelOnTab:nil];
}

// tab is a name from Go's whitelist, or nil to stay on the tab the panel is on.
- (void)showPanelOnTab:(NSString *)tab {
    if (!self.panelWeb) {
        // Created on first show: until then the process has no WebKit helpers. Kept until exit.
        NSString *escaped = [tab stringByAddingPercentEncodingWithAllowedCharacters:NSCharacterSet.alphanumericCharacterSet];
        self.panelWeb = [self webViewWithMode:@"popover" tab:escaped.length > 0 ? escaped : self.tab];
        self.panel.contentView = [self materialView:NSVisualEffectMaterialPopover
                                             radius:kPanelRadius
                                            hosting:self.panelWeb
                                              frame:NSMakeRect(0, 0, kPanelWidth, kPanelHeight)];
    } else if (tab.length > 0) {
        // Lost if the page is still loading (about 0.1 s after the first show); queue it if that is ever hit.
        [self.panelWeb evaluateJavaScript:[NSString stringWithFormat:@"window.mp.show(%@)", jsonString(tab)] completionHandler:nil];
    }
    NSStatusBarButton *button = [self anchorItem].button;
    // AppKit rounds the frame to whole points, and reports the move before setFrame returns or after.
    self.placed = NSZeroRect;
    [self.panel setFrame:[self panelFrameUnder:button.window] display:YES];
    self.placed = self.panel.frame;
    if (!self.panel.isVisible) {
        self.panel.alphaValue = 0;
    }
    [self.panel makeKeyAndOrderFront:nil];
    [self.panel makeFirstResponder:self.panelWeb];
    [NSAnimationContext runAnimationGroup:^(NSAnimationContext *context) {
        context.duration = kFadeIn;
        self.panel.animator.alphaValue = 1;
    }];
    onMain(^{
        [button highlight:YES];
    });
    [self notifyVisibility];
}

// Where the panel opens, and how far it may be resized there: where the user left it while its top
// edge is still on some screen, else under anchor at the default size.
- (NSRect)panelFrameUnder:(NSWindow *)anchor {
    NSRect saved = NSRectFromString([NSUserDefaults.standardUserDefaults stringForKey:kPanelFrameKey] ?: @"");
    NSRect grip = NSMakeRect(NSMinX(saved), NSMaxY(saved) - kGripHeight, NSWidth(saved), kGripHeight);
    NSScreen *screen = anchor.screen ?: NSScreen.mainScreen;
    BOOL moved = NO;
    for (NSScreen *candidate in NSScreen.screens) {
        NSRect seen = NSIntersectionRect(grip, candidate.visibleFrame);
        if (NSWidth(seen) >= kGripWidth && NSHeight(seen) >= kGripHeight) {
            screen = candidate;
            moved = YES;
        }
    }
    NSRect visible = screen.visibleFrame;
    self.panel.minSize = NSMakeSize(kPanelWidth, MIN(kPanelHeight, visible.size.height - 2 * kEdgeGap));
    self.panel.maxSize = visible.size;
    if (moved) {
        // The screen may have shrunk since; the top left corner stays where it was.
        CGFloat width = MAX(self.panel.minSize.width, MIN(NSWidth(saved), NSWidth(visible)));
        CGFloat height = MAX(self.panel.minSize.height, MIN(NSHeight(saved), NSHeight(visible)));
        return NSMakeRect(NSMinX(saved), NSMaxY(saved) - height, width, height);
    }
    CGFloat height = self.panel.minSize.height;
    CGFloat x = NSMidX(anchor.frame) - kPanelWidth / 2;
    x = MAX(NSMinX(visible) + kEdgeGap, MIN(x, NSMaxX(visible) - kPanelWidth - kEdgeGap));
    return NSMakeRect(x, NSMaxY(visible) - height - 5, kPanelWidth, height);
}

// AppKit moving the open panel itself (a display unplugged under it) is remembered like
// a drag; compare against the screen change notification if that ever bites. Reset undoes it.
- (void)rememberPanelFrame:(NSNotification *)note {
    if (note.object == self.panel && !NSIsEmptyRect(self.placed) && !NSEqualRects(self.panel.frame, self.placed)) {
        [NSUserDefaults.standardUserDefaults setObject:NSStringFromRect(self.panel.frame) forKey:kPanelFrameKey];
    }
}

- (void)windowDidMove:(NSNotification *)note {
    [self rememberPanelFrame:note];
}

- (void)windowDidResize:(NSNotification *)note {
    [self rememberPanelFrame:note];
}

- (void)closePanel {
    if (!self.panel.isVisible) {
        return;
    }
    [self.panel orderOut:nil];
    [self.anchor.button highlight:NO];
    self.pinned = NO;
    self.closedAt = CFAbsoluteTimeGetCurrent();
    [self notifyVisibility];
}

- (void)openWindow {
    if (!self.window) {
        NSRect frame = NSMakeRect(0, 0, kWindowWidth, kWindowHeight);
        self.window = [[NSWindow alloc] initWithContentRect:frame
                                                  styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable |
                                                            NSWindowStyleMaskResizable | NSWindowStyleMaskFullSizeContentView
                                                    backing:NSBackingStoreBuffered
                                                      defer:NO];
        self.window.title = @"mac-pulse";
        self.window.titleVisibility = NSWindowTitleHidden;
        self.window.titlebarAppearsTransparent = YES;
        self.window.releasedWhenClosed = NO;
        self.window.contentMinSize = NSMakeSize(kWindowMinWidth, kWindowMinHeight);
        self.window.delegate = self;
        self.window.appearance = forcedAppearance;
        self.windowWeb = [self webViewWithMode:@"window" tab:self.tab];
        self.window.contentView = [self materialView:NSVisualEffectMaterialSidebar radius:0 hosting:self.windowWeb frame:frame];
        [self.window center];
    }
    [self closePanel];
    [NSApp activateIgnoringOtherApps:YES];
    self.window.level = windowOnTop ? NSFloatingWindowLevel : NSNormalWindowLevel;
    [self.window makeKeyAndOrderFront:nil];
    applyDock();
    [self notifyVisibility];
}

- (void)windowDidResignKey:(NSNotification *)note {
    if (note.object == self.panel && !self.pinned) {
        [self closePanel];
    }
}

- (void)windowWillClose:(NSNotification *)note {
    // isVisible is still YES inside windowWillClose.
    onMain(^{
        applyDock();
        [self notifyVisibility];
    });
}

- (void)windowDidMiniaturize:(NSNotification *)note {
    [self notifyVisibility];
}

- (void)windowDidDeminiaturize:(NSNotification *)note {
    [self notifyVisibility];
}

- (void)notifyVisibility {
    // A window in the Dock is still isVisible, and nobody is looking at it.
    BOOL window = self.window.isVisible && !self.window.isMiniaturized;
    NSString *json = [NSString stringWithFormat:@"{\"type\":\"visibility\",\"popover\":%@,\"window\":%@,\"pinned\":%@}",
                                                self.panel.isVisible ? @"true" : @"false", window ? @"true" : @"false",
                                                self.pinned ? @"true" : @"false"];
    mpMessage((char *)json.UTF8String);
}

- (void)eval:(NSString *)js {
    if (self.panel.isVisible) {
        [self.panelWeb evaluateJavaScript:js completionHandler:nil];
    }
    if (self.window.isVisible) {
        [self.windowWeb evaluateJavaScript:js completionHandler:nil];
    }
}

// Writes the web view on screen (the window if open, else the panel) to a PNG; failure is nil on success.
- (void)capture:(NSString *)path options:(NSDataWritingOptions)options completion:(void (^)(NSString *failure))completion {
    WKWebView *web = self.window.isVisible ? self.windowWeb : self.panelWeb;
    if (!web) {
        completion(@"no view has been opened");
        return;
    }
    [web takeSnapshotWithConfiguration:nil
                     completionHandler:^(NSImage *image, NSError *err) {
                         if (!image) {
                             completion(err.localizedDescription ?: @"no image");
                             return;
                         }
                         // Approximates the blurred material the web view sits on.
                         NSColor *fill = isDark(web) ? [NSColor colorWithWhite:0.16 alpha:1] : [NSColor colorWithWhite:0.93 alpha:1];
                         completion(writePNG(image, fill, path, options).localizedDescription);
                     }];
}

- (void)snapshot:(NSString *)path {
    [self capture:path
          options:NSDataWritingAtomic
       completion:^(NSString *failure) {
           if (failure) {
               NSLog(@"mac-pulse: snapshot %@: %@", path, failure);
           }
       }];
    // The main item, when there is one, as <path>-status.png, every extra one as <path>-status-<id>.png.
    NSMutableDictionary<NSString *, NSStatusItem *> *shown = [NSMutableDictionary new];
    shown[@"-status.png"] = self.item;
    for (NSString *name in self.extras) {
        shown[[NSString stringWithFormat:@"-status-%@.png", name]] = self.extras[name];
    }
    for (NSString *suffix in shown) {
        NSStatusBarButton *button = shown[suffix].button;
        NSBitmapImageRep *rep = [button bitmapImageRepForCachingDisplayInRect:button.bounds];
        [button cacheDisplayInRect:button.bounds toBitmapImageRep:rep];
        NSImage *status = [[NSImage alloc] initWithSize:button.bounds.size];
        [status addRepresentation:rep];
        NSString *statusPath = [path.stringByDeletingPathExtension stringByAppendingString:suffix];
        NSError *err = writePNG(status, isDark(button) ? [NSColor colorWithWhite:0.2 alpha:1] : [NSColor colorWithWhite:0.85 alpha:1],
                                statusPath, NSDataWritingAtomic);
        if (err) {
            NSLog(@"mac-pulse: snapshot %@: %@", statusPath, err);
        }
    }
}

- (void)userContentController:(WKUserContentController *)controller didReceiveScriptMessage:(WKScriptMessage *)message {
    if (!message.frameInfo.isMainFrame || ![message.frameInfo.securityOrigin.protocol isEqualToString:@"mp"]) {
        return;
    }
    // Go validates the meaning; here only the shape: one JSON object, or a string that claims to be one.
    NSString *json;
    if ([message.body isKindOfClass:NSString.class]) {
        json = message.body;
    } else if ([message.body isKindOfClass:NSDictionary.class] && [NSJSONSerialization isValidJSONObject:message.body]) {
        NSData *data = [NSJSONSerialization dataWithJSONObject:message.body options:0 error:nil];
        json = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
    }
    if (json.length == 0 || json.length > kMaxMessageLength) {
        return;
    }
    mpMessage((char *)json.UTF8String);
}

// Icons are rendered on first use, so bodies are read off the main thread.
- (void)webView:(WKWebView *)webView startURLSchemeTask:(id<WKURLSchemeTask>)task {
    [self.tasks addObject:task];
    NSURL *url = task.request.URL;
    dispatch_async(dispatch_get_global_queue(QOS_CLASS_USER_INITIATED, 0), ^{
        int len = 0;
        char *mime = NULL;
        void *bytes = mpAsset((char *)url.path.UTF8String, (char *)(url.query ?: @"").UTF8String, &len, &mime);
        NSData *body = bytes ? [NSData dataWithBytesNoCopy:bytes length:len freeWhenDone:YES] : nil;
        NSString *type = mime ? @(mime) : @"";
        free(mime);
        onMain(^{
            if (![self.tasks containsObject:task]) {
                return;
            }
            [self.tasks removeObject:task];
            if (!body) {
                [task didFailWithError:[NSError errorWithDomain:NSURLErrorDomain code:NSURLErrorFileDoesNotExist userInfo:nil]];
                return;
            }
            NSDictionary *headers = @{
                @"Content-Type": type,
                @"Content-Length": @(body.length).stringValue,
                @"Cache-Control": @"no-store",
                @"Content-Security-Policy": @"default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'",
            };
            [task didReceiveResponse:[[NSHTTPURLResponse alloc] initWithURL:url statusCode:200 HTTPVersion:@"HTTP/1.1" headerFields:headers]];
            [task didReceiveData:body];
            [task didFinish];
        });
    });
}

- (void)webView:(WKWebView *)webView stopURLSchemeTask:(id<WKURLSchemeTask>)task {
    [self.tasks removeObject:task];
}

// The web views only ever show the embedded frontend; anything else (a link in a process name) is dropped.
- (void)webView:(WKWebView *)webView
    decidePolicyForNavigationAction:(WKNavigationAction *)action
                    decisionHandler:(void (^)(WKNavigationActionPolicy))decisionHandler {
    NSURL *url = action.request.URL;
    BOOL ours = [url.scheme isEqualToString:@"mp"] || [url.absoluteString isEqualToString:@"about:blank"];
    decisionHandler(ours ? WKNavigationActionPolicyAllow : WKNavigationActionPolicyCancel);
}

@end

void mpRun(const char *tab, const char *version, int open, int window, int debug) {
    @autoreleasepool {
        debugMode = debug;
        NSApplication *app = NSApplication.sharedApplication;
        app.activationPolicy = NSApplicationActivationPolicyAccessory;
        shell = [MPShell new];
        shell.tab = @(tab);
        shell.version = @(version);
        shell.openAtStart = open;
        shell.windowAtStart = window;
        app.delegate = shell;
        [app run];
        [shell removeItem:nil];
        for (NSString *name in shell.extras.allKeys) {
            [shell removeItem:name];
        }
    }
}

void mpQuit(void) {
    atomic_store(&quitting, true);
    onMain(^{
        [folderPanel cancel:nil];
        [NSApp stop:nil];
        // stop: only takes effect once the run loop handles another event.
        [NSApp postEvent:[NSEvent otherEventWithType:NSEventTypeApplicationDefined
                                            location:NSZeroPoint
                                       modifierFlags:0
                                           timestamp:0
                                        windowNumber:0
                                             context:nil
                                             subtype:0
                                               data1:0
                                               data2:0]
                 atStart:YES];
    });
}

void mpSetStatus(const char *json) {
    NSData *data = [NSData dataWithBytes:json length:strlen(json)];
    onMain(^{
        NSArray *items = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
        if ([items isKindOfClass:NSArray.class]) {
            [shell setStatus:items];
        }
    });
}

void mpSetExtraStatus(const char *name, const char *tab, const char *json) {
    NSString *item = @(name), *opens = @(tab);
    NSData *data = [NSData dataWithBytes:json length:strlen(json)];
    onMain(^{
        NSArray *items = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
        if (item.length > 0 && [items isKindOfClass:NSArray.class]) {
            [shell setExtra:item tab:opens items:items];
        }
    });
}

void mpSetClock(const char *format, const char *locale) {
    NSString *template = @(format), *language = @(locale);
    onMain(^{
        [shell setClock:template locale:language];
    });
}

// The app is a regular one (Dock icon, Cmd+Tab) when the user asked for the Dock, and for as
// long as its window is open or minimised: a window that Cmd+Tab cannot reach gets lost.
static void applyDock(void) {
    BOOL regular = dockSetting || shell.window.isVisible || shell.window.isMiniaturized;
    // A bare binary has no bundle icon and would sit in the Dock as a generic executable.
    // The embedded icon is 64 px and soft at Dock size; the bundle has the full icon set.
    if (regular && ![NSBundle.mainBundle objectForInfoDictionaryKey:@"CFBundleIconFile"]) {
        int len = 0;
        char *mime = NULL;
        void *bytes = mpAsset("/icon", "path=self", &len, &mime);
        free(mime);
        if (bytes) {
            NSApp.applicationIconImage = [[NSImage alloc] initWithData:[NSData dataWithBytesNoCopy:bytes length:len freeWhenDone:YES]];
        }
    }
    NSApplicationActivationPolicy policy = regular ? NSApplicationActivationPolicyRegular : NSApplicationActivationPolicyAccessory;
    if (NSApp.activationPolicy != policy) {
        [NSApp setActivationPolicy:policy];
    }
}

void mpSetDock(int show) {
    onMain(^{
        dockSetting = show;
        applyDock();
    });
}

char *mpChooseFolder(void) {
    dispatch_semaphore_t done = dispatch_semaphore_create(0);
    __block NSString *path = nil;
    onMain(^{
        BOOL reopen = shell.panel.isVisible;
        folderPanel = [NSOpenPanel openPanel];
        folderPanel.canChooseFiles = NO;
        folderPanel.canChooseDirectories = YES;
        folderPanel.allowsMultipleSelection = NO;
        // A pinned panel stays up at the menu level and would cover the dialog.
        folderPanel.level = NSPopUpMenuWindowLevel;
        [NSApp activateIgnoringOtherApps:YES];
        [folderPanel beginWithCompletionHandler:^(NSModalResponse response) {
            if (response == NSModalResponseOK) {
                path = folderPanel.URL.path;
            }
            folderPanel = nil;
            // The dialog took the key status, which closes an unpinned panel, and the scan shows there.
            if (reopen && !shell.window.isVisible && !atomic_load(&quitting)) {
                [shell showPanel];
            }
            dispatch_semaphore_signal(done);
        }];
    });
    // Polled rather than awaited: once the app quits, the block above may never run.
    while (dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, kChooseFolderPoll)) != 0) {
        if (atomic_load(&quitting)) {
            return NULL;
        }
    }
    return path ? strdup(path.UTF8String) : NULL;
}

void mpSetStatusStyle(int compact) {
    onMain(^{
        compactStatus = compact;
        [shell setStatus:shell.statusItems];
        for (NSString *name in shell.extras) {
            [shell draw:shell.extraItems[name] on:shell.extras[name]];
        }
    });
}

void mpSetMenu(const char *open, const char *window, const char *export, const char *settings, const char *quit, const char *reset) {
    // @() yields nil for invalid UTF-8, and nil in an array literal throws.
    NSArray<NSString *> *titles = @[@(open) ?: @"", @(window) ?: @"", @(export) ?: @"", @(settings) ?: @"", @(quit) ?: @"", @(reset) ?: @""];
    onMain(^{
        menuTitles = titles;
    });
}

char *mpPreferredLanguages(void) {
    return strdup([NSLocale.preferredLanguages componentsJoinedByString:@","].UTF8String);
}

void mpEval(const char *js) {
    NSString *script = @(js);
    onMain(^{
        [shell eval:script];
    });
}

void mpBackToPanel(void) {
    onMain(^{
        [shell.window close];
        applyDock();
        // Leaving the Dock deactivates the app a moment later, and a panel shown before that
        // would lose key and close again; it is shown once the switch has settled.
        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(0.35 * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
            [shell showPanelOnTab:@""];
        });
    });
}

void mpShowPanel(const char *tab) {
    NSString *name = @(tab);
    onMain(^{
        [shell showPanelOnTab:name];
    });
}

void mpSetPinned(int on) {
    onMain(^{
        shell.pinned = on;
        [shell notifyVisibility];
    });
}

void mpSetWindowOnTop(int on) {
    onMain(^{
        windowOnTop = on;
        shell.window.level = on ? NSFloatingWindowLevel : NSNormalWindowLevel;
    });
}

void mpSetAppearance(int mode) {
    onMain(^{
        forcedAppearance = mode == 0 ? nil : [NSAppearance appearanceNamed:mode == 2 ? NSAppearanceNameDarkAqua : NSAppearanceNameAqua];
        shell.panel.appearance = forcedAppearance;
        shell.window.appearance = forcedAppearance;
    });
}

void mpOpenWindow(void) {
    onMain(^{
        [shell openWindow];
    });
}

void mpSnapshot(const char *path) {
    NSString *file = @(path);
    onMain(^{
        [shell snapshot:file];
    });
}

char *mpExport(const char *path) {
    NSString *file = @(path);
    dispatch_semaphore_t done = dispatch_semaphore_create(0);
    __block NSString *failure = nil;
    onMain(^{
        if (!shell.panel.isVisible && !shell.window.isVisible) {
            failure = @"no view is open";
            dispatch_semaphore_signal(done);
            return;
        }
        [shell capture:file
               options:NSDataWritingWithoutOverwriting
            completion:^(NSString *err) {
                failure = err;
                if (!err) {
                    [NSWorkspace.sharedWorkspace activateFileViewerSelectingURLs:@[[NSURL fileURLWithPath:file]]];
                }
                dispatch_semaphore_signal(done);
            }];
    });
    if (dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, kExportTimeout)) != 0) {
        return strdup("timed out");
    }
    return failure ? strdup(failure.UTF8String) : NULL;
}

void mpCopy(const char *text) {
    NSString *string = @(text);
    onMain(^{
        [NSPasteboard.generalPasteboard clearContents];
        [NSPasteboard.generalPasteboard setString:string ?: @"" forType:NSPasteboardTypeString];
    });
}

static OSStatus onHotKey(EventHandlerCallRef next, EventRef event, void *context) {
    EventHotKeyID key = {0};
    GetEventParameter(event, kEventParamDirectObject, typeEventHotKeyID, NULL, sizeof key, NULL, &key);
    if (key.id == 1) {
        mpMessage("{\"type\":\"hotkey\",\"name\":\"quit\"}");
    } else {
        [shell togglePanel];
    }
    return noErr;
}

// Carbon is deprecated but remains the only global shortcut API that needs no Accessibility permission.
int mpSetHotkey(int id, int on) {
    static EventHotKeyRef hotKeys[2];
    static const UInt32 keyCodes[2] = {kVK_ANSI_P, kVK_ANSI_K};
    static BOOL handlerInstalled;
    __block OSStatus status = noErr;
    dispatch_block_t apply = ^{
        if (!on) {
            if (hotKeys[id]) {
                UnregisterEventHotKey(hotKeys[id]);
                hotKeys[id] = NULL;
            }
            return;
        }
        if (hotKeys[id]) {
            return;
        }
        // The event target exists only once there is an application; mpRun may not have run yet.
        [NSApplication sharedApplication];
        if (!handlerInstalled) {
            EventTypeSpec spec = {kEventClassKeyboard, kEventHotKeyPressed};
            status = InstallEventHandler(GetApplicationEventTarget(), onHotKey, 1, &spec, NULL, NULL);
            if (status != noErr) {
                return;
            }
            handlerInstalled = YES;
        }
        EventHotKeyID hotKeyID = {.signature = 'mpls', .id = id};
        status = RegisterEventHotKey(keyCodes[id], controlKey | optionKey, hotKeyID, GetApplicationEventTarget(), 0, &hotKeys[id]);
        if (status != noErr) {
            hotKeys[id] = NULL;
        }
    };
    if (NSThread.isMainThread) {
        apply();
        return status;
    }
    dispatch_semaphore_t done = dispatch_semaphore_create(0);
    onMain(^{
        apply();
        dispatch_semaphore_signal(done);
    });
    if (dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, kMainTimeout)) != 0) {
        return kMPTimeoutErr;
    }
    return status;
}

// terminate: is the polite AppKit quit (Cmd+Q with its "Save changes?" dialog); it needs the
// main queue and answers NO for a process that is not an application.
int mpTerminateApp(int pid) {
    dispatch_semaphore_t done = dispatch_semaphore_create(0);
    __block BOOL sent = NO;
    onMain(^{
        NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
        sent = app != nil && [app terminate];
        dispatch_semaphore_signal(done);
    });
    // A main queue that does not answer means mac-pulse is quitting. Answering 0 would make the
    // caller signal the app, which skips its "Save changes?" dialog, so the request counts as made.
    if (dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, kMainTimeout)) != 0) {
        return 1;
    }
    return sent;
}

void *mpAppIcon(const char *path, int *len) {
    @autoreleasepool {
        NSImage *icon = [NSWorkspace.sharedWorkspace iconForFile:@(path)];
        NSBitmapImageRep *rep = [[NSBitmapImageRep alloc] initWithBitmapDataPlanes:NULL
                                                                        pixelsWide:64
                                                                        pixelsHigh:64
                                                                     bitsPerSample:8
                                                                   samplesPerPixel:4
                                                                          hasAlpha:YES
                                                                          isPlanar:NO
                                                                    colorSpaceName:NSCalibratedRGBColorSpace
                                                                       bytesPerRow:0
                                                                      bitsPerPixel:0];
        [NSGraphicsContext saveGraphicsState];
        NSGraphicsContext.currentContext = [NSGraphicsContext graphicsContextWithBitmapImageRep:rep];
        [icon drawInRect:NSMakeRect(0, 0, 64, 64) fromRect:NSZeroRect operation:NSCompositingOperationCopy fraction:1];
        [NSGraphicsContext restoreGraphicsState];
        NSData *png = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
        void *out = malloc(png.length);
        memcpy(out, png.bytes, png.length);
        *len = (int)png.length;
        return out;
    }
}

void mpNotify(const char *title, const char *body, const char *tab) {
    // @() yields nil for invalid UTF-8 (a process name), and nil in an array literal throws.
    NSString *heading = @(title) ?: @"mac-pulse", *text = @(body) ?: @"", *onClick = @(tab) ?: @"";
    onMain(^{
        // The deprecated centre is the only one that accepts an ad-hoc signed bundle
        // (UNUserNotificationCenter answers "Notifications are not allowed for this application").
        // Move to UserNotifications once the app ships with a Developer ID.
        NSUserNotificationCenter *center = NSUserNotificationCenter.defaultUserNotificationCenter;
        if (center) {
            NSUserNotification *notification = [NSUserNotification new];
            notification.title = heading;
            notification.informativeText = text;
            notification.userInfo = @{@"tab": onClick};
            [center deliverNotification:notification];
            return;
        }
        // No bundle (make run): the banner comes from Script Editor. The text travels as argv, never as script source.
        NSArray *args = @[
            @"-e", @"on run argv", @"-e", @"display notification (item 1 of argv) with title (item 2 of argv)", @"-e", @"end run", @"--", text,
            heading
        ];
        NSError *err;
        if (![NSTask launchedTaskWithExecutableURL:[NSURL fileURLWithPath:@"/usr/bin/osascript"] arguments:args error:&err terminationHandler:nil]) {
            NSLog(@"mac-pulse: notify: %@", err);
        }
    });
}
