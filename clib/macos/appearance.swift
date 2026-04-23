import Cocoa

// Compilation: swiftc appearance.swift -o appearance
// Usage: ./appearance

func getAccentColor() -> String {
    if #available(macOS 10.14, *) {
        let color = NSColor.controlAccentColor.usingColorSpace(.sRGB)
        if let color = color {
            let r = Int(color.redComponent * 255)
            let g = Int(color.greenComponent * 255)
            let b = Int(color.blueComponent * 255)
            return String(format: "#%02X%02X%02X", r, g, b)
        }
    }
    return "#007AFF" // Default macOS blue
}

func isDarkMode() -> Bool {
    let mode = UserDefaults.standard.string(forKey: "AppleInterfaceStyle")
    return mode == "Dark"
}

print("\(isDarkMode()) \(getAccentColor())")
