import Cocoa

// Compilation: swiftc file_picker.swift -o file_picker
// Usage: ./file_picker [initial_path]

func openFilePicker(initialPath: String?) {
    let dialog = NSOpenPanel()
    
    dialog.title                   = "Select an attachment"
    dialog.showsResizeIndicator    = true
    dialog.showsHiddenFiles        = false
    dialog.canChooseDirectories    = false
    dialog.canCreateDirectories    = false
    dialog.allowsMultipleSelection = true
    
    if let initialPath = initialPath {
        dialog.directoryURL = URL(fileURLWithPath: (initialPath as NSString).expandingTildeInPath)
    }

    // Since this is a CLI helper, we need to force it to the front
    NSApp.setActivationPolicy(.accessory)
    NSApp.activate(ignoringOtherApps: true)

    if dialog.runModal() == .OK {
        let results = dialog.urls.map { $0.path }
        print(results.joined(separator: "\n"))
    }
}

let args = ProcessInfo.processInfo.arguments
let initialPath = args.count > 1 ? args[1] : nil

openFilePicker(initialPath: initialPath)
