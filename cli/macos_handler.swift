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
            let script = "tell application \"Terminal\" to do script \"'\(matchaPath)' '\(urlString)'\""
            
            if let appleScript = NSAppleScript(source: script) {
                appleScript.executeAndReturnError(nil)
            }
        }

        DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) {
            NSApp.terminate(nil)
        }
    }
}

let delegate = AppDelegate()
NSApplication.shared.delegate = delegate
NSApplication.shared.run()
