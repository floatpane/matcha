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
        
        // Timeout
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
        if let urlString = event.paramDescriptor(forKeyword: AEKeyword(757935405))?.stringValue {
            log("Legacy API received URL: \(urlString)")
            launchMatcha(with: urlString)
        }
    }
    
    func launchMatcha(with url: String) {
        guard !handled else { return }
        handled = true
        
        let matchaPath = "{{MATCHA_PATH}}"
        log("Launching Matcha via .command file at \(matchaPath) with URL \(url)")
        
        // Use a .command file to open in the DEFAULT terminal
        let tempDir = NSTemporaryDirectory()
        let commandFileName = "matcha-mailto-\(UUID().uuidString).command"
        let commandFileUrl = URL(fileURLWithPath: tempDir).appendingPathComponent(commandFileName)
        
        // We use a bash script that opens matcha and then removes itself
        let scriptContent = """
        #!/bin/bash
        '\(matchaPath)' '\(url)'
        # Clean up this temporary script
        rm -- "$0"
        exit
        """
        
        do {
            try scriptContent.write(to: commandFileUrl, atomically: true, encoding: .utf8)
            
            // Make the file executable
            let attributes = [FileAttributeKey.posixPermissions: 0o755]
            try FileManager.default.setAttributes(attributes, ofItemAtPath: commandFileUrl.path)
            
            // Open the file with NSWorkspace. 
            // Since it's a .command file, macOS will open it in the default terminal.
            NSWorkspace.shared.open(commandFileUrl)
            log("Successfully requested macOS to open .command file")
            
        } catch {
            log("Failed to create/open .command file: \(error.localizedDescription)")
        }
        
        // Small delay to ensure launch
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
