import Cocoa

class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)

        // kInternetEventClass = 'GURL' (0x4755524c)
        // kAEGetURL = 'GURL' (0x4755524c)
        let eventClass = AEEventClass(0x4755524c)
        let eventID = AEEventID(0x4755524c)

        // Register for the 'getURL' event
        NSAppleEventManager.shared().setEventHandler(
            self,
            andSelector: #selector(handleGetURLEvent(_:withReplyEvent:)),
            forEventClass: eventClass,
            andEventID: eventID
        )
    }

    @objc func handleGetURLEvent(_ event: NSAppleEventDescriptor, withReplyEvent replyEvent: NSAppleEventDescriptor) {
        // keyDirectObject = '----' (0x2d2d2d2d)
        guard let urlString = event.paramDescriptor(forKeyword: AEKeyword(0x2d2d2d2d))?.stringValue,
              let url = URL(string: urlString) else {
            return
        }
...

        let matchaPath = "{{MATCHA_PATH}}"
        
        // Use AppleScript to tell Terminal to run matcha with the URL
        // We escape the URL and path for shell safety
        let script = """
        tell application "Terminal"
            activate
            do script "'\(matchaPath)' '\(url.absoluteString)'"
        end tell
        """

        var error: NSDictionary?
        if let appleScript = NSAppleScript(source: script) {
            appleScript.executeAndReturnError(&error)
        }

        // Exit after handling the event
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) {
            NSApplication.shared.terminate(nil)
        }
    }
}

let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
app.run()
