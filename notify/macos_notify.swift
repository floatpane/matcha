#!/usr/bin/swift
import Cocoa

// Compilation: swiftc macos_notify.swift -o macos_notify
// Usage: ./macos_notify <title> <body> <logoPath> [subtitle] [identifier] [soundName] [contentImagePath]
// Or: ./macos_notify --remove [identifier]

let args = ProcessInfo.processInfo.arguments

if args.contains("--remove") {
    let center = NSUserNotificationCenter.default
    if args.count > 2 {
        let identifier = args[2]
        center.removeDeliveredNotification(withIdentifier: identifier)
    } else {
        center.removeAllDeliveredNotifications()
    }
    exit(0)
}

guard args.count >= 4 else {
    print("Usage: \(args[0]) <title> <body> <logoPath> [subtitle] [identifier] [soundName] [contentImagePath]")
    print("       \(args[0]) --remove [identifier]")
    exit(1)
}

let title = args[1]
let body = args[2]
let logoPath = args[3]
let subtitle = args.count > 4 ? args[4] : ""
let identifier = args.count > 5 ? args[5] : nil
let soundName = args.count > 6 ? args[6] : NSUserNotificationDefaultSoundName
let contentImagePath = args.count > 7 ? args[7] : ""

class NotificationDelegate: NSObject, NSUserNotificationCenterDelegate {
    func userNotificationCenter(_ center: NSUserNotificationCenter, shouldPresent notification: NSUserNotification) -> Bool {
        return true
    }
}

let notification = NSUserNotification()
notification.title = title
notification.informativeText = body
notification.subtitle = subtitle
notification.soundName = soundName

if let identifier = identifier {
    notification.identifier = identifier
}

func loadImage(from path: String) -> NSImage? {
    if path.isEmpty { return nil }
    let expandedPath = (path as NSString).expandingTildeInPath
    return NSImage(contentsOfFile: expandedPath)
}

if let img = loadImage(from: logoPath) {
    // _identityImage is a private key that allows showing an image on the left of the notification.
    notification.setValue(img, forKey: "_identityImage")
}

if let contentImg = loadImage(from: contentImagePath) {
    notification.contentImage = contentImg
}

let delegate = NotificationDelegate()
NSUserNotificationCenter.default.delegate = delegate
NSUserNotificationCenter.default.deliver(notification)

// Wait for the notification to be delivered to the system.
RunLoop.main.run(until: Date(timeIntervalSinceNow: 0.1))
