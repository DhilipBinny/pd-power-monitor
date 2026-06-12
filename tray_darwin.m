#import <Cocoa/Cocoa.h>
#include "tray_darwin.h"

extern void goOnUpdate(void);

@interface TrayDelegate : NSObject
@end

@implementation TrayDelegate
- (void)quit:(id)sender { tray_quit(); }
- (void)tick:(NSTimer *)timer { goOnUpdate(); }
@end

static NSStatusItem *statusItem;
static NSMenu *menu;
static NSMutableArray<NSMenuItem *> *portItems;
static NSMenuItem *batItem, *totalItem;
static TrayDelegate *delegate;

// Port rows are inserted after "Power Monitor" + separator.
static const NSInteger portInsertIndex = 2;

static NSMenuItem *label_item(NSString *title) {
	NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:title
	                                              action:nil
	                                       keyEquivalent:@""];
	item.enabled = NO;
	return item;
}

void tray_init(void) {
	[NSApplication sharedApplication];
	[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];

	delegate = [[TrayDelegate alloc] init];
	portItems = [[NSMutableArray alloc] init];

	statusItem = [[NSStatusBar systemStatusBar]
		statusItemWithLength:NSVariableStatusItemLength];
	if (@available(macOS 11.0, *)) {
		NSImage *icon = [NSImage imageWithSystemSymbolName:@"bolt.fill"
		                          accessibilityDescription:@"Power Monitor"];
		if (icon) {
			statusItem.button.image = icon;
			statusItem.button.imagePosition = NSImageLeft;
		}
	}

	menu = [[NSMenu alloc] init];
	menu.autoenablesItems = NO;

	[menu addItem:label_item(@"Power Monitor")];
	[menu addItem:[NSMenuItem separatorItem]];
	// port rows are inserted here dynamically
	[menu addItem:[NSMenuItem separatorItem]];

	batItem = label_item(@"Battery: --");
	[menu addItem:batItem];
	totalItem = label_item(@"Power input: --");
	[menu addItem:totalItem];

	[menu addItem:[NSMenuItem separatorItem]];
	NSMenuItem *quit = [[NSMenuItem alloc] initWithTitle:@"Quit"
	                                              action:@selector(quit:)
	                                       keyEquivalent:@"q"];
	quit.target = delegate;
	[menu addItem:quit];

	statusItem.menu = menu;
}

void tray_set_port_count(int count) {
	for (NSMenuItem *item in portItems)
		[menu removeItem:item];
	[portItems removeAllObjects];
	for (int i = 0; i < count; i++) {
		NSMenuItem *item = label_item(@"");
		[menu insertItem:item atIndex:portInsertIndex + i];
		[portItems addObject:item];
	}
}

void tray_set_port_label(int i, const char *s) {
	if (i >= 0 && i < (int)portItems.count)
		portItems[i].title = [NSString stringWithUTF8String:s];
}

void tray_set_bat(const char *s) {
	batItem.title = [NSString stringWithUTF8String:s];
}

void tray_set_total(const char *s) {
	totalItem.title = [NSString stringWithUTF8String:s];
}

void tray_set_title(const char *s) {
	statusItem.button.title = [NSString stringWithUTF8String:s];
}

void tray_run(double interval) {
	NSTimer *timer = [NSTimer timerWithTimeInterval:interval
	                                         target:delegate
	                                       selector:@selector(tick:)
	                                       userInfo:nil
	                                        repeats:YES];
	// CommonModes so the menu keeps refreshing while open
	[[NSRunLoop mainRunLoop] addTimer:timer forMode:NSRunLoopCommonModes];
	[NSApp run];
}

void tray_quit(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		[NSApp stop:nil];
		// stop: only takes effect after an event is processed; post one
		NSEvent *e = [NSEvent otherEventWithType:NSEventTypeApplicationDefined
		                                location:NSZeroPoint
		                           modifierFlags:0
		                               timestamp:0
		                            windowNumber:0
		                                 context:nil
		                                 subtype:0
		                                   data1:0
		                                   data2:0];
		[NSApp postEvent:e atStart:YES];
	});
}
