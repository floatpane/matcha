import Cocoa

// Compilation: swiftc badge.swift -o badge
// Usage: ./badge <count>

let args = ProcessInfo.processInfo.arguments
guard args.count > 1 else {
    print("Usage: \(args[0]) <count>")
    exit(1)
}

let countString = args[1]
let label = countString == "0" ? "" : countString

// Note: This only works if the process has a Dock icon (is a bundled app or has activation policy set)
// For a CLI tool, this would set the badge of the Terminal app if run directly,
// which might not be what's intended. 
// However, if we run this from our 'MatchaMail.app' shim, it will work for that app.

NSApp.dockTile.badgeLabel = label
NSApp.dockTile.display()

// Give it a tiny bit of time to propagate
RunLoop.main.run(until: Date(timeIntervalSinceNow: 0.1))
