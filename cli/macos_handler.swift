import Cocoa

class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory) // hide from dock initially
    }
    func application(_ application: NSApplication, open urls: [URL]) {
        guard let url = urls.first else { return }

        let matchaPath = "{{MATCHA_PATH}}"
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

        NSApplication.shared.terminate(nil)
    }
}

let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
app.run()
