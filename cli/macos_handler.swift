import Cocoa

class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        // 1196711500 = 'GURL'
        NSAppleEventManager.shared().setEventHandler(
            self,
            andSelector: #selector(handleGetURLEvent(_:withReplyEvent:)),
            forEventClass: AEEventClass(1196711500),
            andEventID: AEEventID(1196711500)
        )
    }
    
    @objc func handleGetURLEvent(_ event: NSAppleEventDescriptor, withReplyEvent replyEvent: NSAppleEventDescriptor) {
        // 757935405 = '----' (keyDirectObject)
        if let urlString = event.paramDescriptor(forKeyword: AEKeyword(757935405))?.stringValue {
            let matchaPath = "{{MATCHA_PATH}}"
            
            // Expanded AppleScript for better reliability
            let scriptSource = """
            tell application "Terminal"
                activate
                do script "'\(matchaPath)' '\(urlString)'"
            end tell
            """
            
            if let appleScript = NSAppleScript(source: scriptSource) {
                var error: NSDictionary?
                appleScript.executeAndReturnError(&error)
                if let err = error {
                    // Log to console if there's an error (can be seen in Console.app)
                    NSLog("MatchaMail AppleScript Error: \(err)")
                }
            }
        }

        // Increased delay slightly to ensure the Apple Event is sent successfully
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) {
            NSApp.terminate(nil)
        }
    }
}

let delegate = AppDelegate()
NSApplication.shared.delegate = delegate
NSApplication.shared.run()
