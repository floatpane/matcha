import Cocoa

class AppDelegate: NSObject, NSApplicationDelegate {
    var handled = false
    
    func applicationDidFinishLaunching(_ notification: Notification) {
        log("MatchaMail handler started")
        
        // Register for legacy Apple Events (GURL = 1196711500)
        NSAppleEventManager.shared().setEventHandler(
            self,
            andSelector: #selector(handleGetURLEvent(_:withReplyEvent:)),
            forEventClass: AEEventClass(1196711500),
            andEventID: AEEventID(1196711500)
        )
        
        // If nothing handled in 2 seconds, assume failure/no event and quit
        DispatchQueue.main.asyncAfter(deadline: .now() + 2.0) {
            if !self.handled {
                self.log("No URL event received within 2s, terminating.")
                NSApp.terminate(nil)
            }
        }
    }
    
    // Modern URL handling
    func application(_ application: NSApplication, open urls: [URL]) {
        if let url = urls.first {
            log("Modern API received URL: \(url.absoluteString)")
            launchMatcha(with: url.absoluteString)
        }
    }
    
    // Legacy Apple Event handling
    @objc func handleGetURLEvent(_ event: NSAppleEventDescriptor, withReplyEvent replyEvent: NSAppleEventDescriptor) {
        // keyDirectObject = 757935405
        if let urlString = event.paramDescriptor(forKeyword: AEKeyword(757935405))?.stringValue {
            log("Legacy API received URL: \(urlString)")
            launchMatcha(with: urlString)
        }
    }
    
    func launchMatcha(with url: String) {
        guard !handled else { return }
        handled = true
        
        let matchaPath = "{{MATCHA_PATH}}"
        log("Launching Matcha at \(matchaPath) with URL \(url)")
        
        // Use AppleScript to tell Terminal to run matcha
        // We use a more robust script that ensures Terminal is active
        let scriptSource = """
        tell application "Terminal"
            activate
            do script "'\(matchaPath)' '\(url)'"
        end tell
        """
        
        if let appleScript = NSAppleScript(source: scriptSource) {
            var error: NSDictionary?
            appleScript.executeAndReturnError(&error)
            if let err = error {
                log("AppleScript Error: \(err)")
            } else {
                log("AppleScript executed successfully")
            }
        }
        
        // Small delay to ensure handoff
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) {
            NSApp.terminate(nil)
        }
    }
    
    func log(_ message: String) {
        let logPath = "/tmp/matcha-handler.log"
        let timestamp = Date().description
        let line = "[\(timestamp)] \(message)\n"
        
        if let data = line.data(using: .utf8) {
            if let fileHandle = FileHandle(forWritingAtPath: logPath) {
                fileHandle.seekToEndOfFile()
                fileHandle.write(data)
                fileHandle.closeFile()
            } else {
                try? data.write(to: URL(fileURLWithPath: logPath))
            }
        }
        NSLog(message)
    }
}

let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
app.run()
