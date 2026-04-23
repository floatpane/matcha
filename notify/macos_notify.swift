import Cocoa

let args = ProcessInfo.processInfo.arguments
if args.count < 4 { exit(1) }

let title = args[1]
let body = args[2]
let logoPath = args[3]

let notification = NSUserNotification()
notification.title = title
notification.informativeText = body
notification.soundName = NSUserNotificationDefaultSoundName

if let img = NSImage(contentsOfFile: logoPath) {
    notification.setValue(img, forKey: "_identityImage")
}

NSUserNotificationCenter.default.deliver(notification)
RunLoop.main.run(until: Date(timeIntervalSinceNow: 0.1))
