import Cocoa
import Darwin
import UserNotifications

// MatchaHelper — a macOS menu bar agent for the Matcha terminal email client.
//
// It connects to the running matcha daemon over its Unix domain socket and
// speaks the same newline-delimited JSON-RPC protocol the TUI uses. From there
// it:
//   * shows the total unread count in the menu bar,
//   * posts native notifications (with the Matcha icon) on new mail,
//   * lists accounts in its dropdown so you can open Matcha in a terminal.
//
// The matcha binary path and notification icon path are templated in by the
// Go installer (cli/helper.go) before compilation.
//
// Compilation (standalone test): replace the two placeholders with literals and
//   swiftc -O macos_menubar.swift -o MatchaHelper

let matchaPath = "{{MATCHA_PATH}}"
let iconPath = "{{ICON_PATH}}"

// Refresh the unread count at most this often as a backstop; event-driven
// refreshes (new mail / sync) keep it current in between.
let backstopRefreshInterval: TimeInterval = 180

let logFilePath = "\(NSHomeDirectory())/Library/Logs/MatchaHelper.log"

func logLine(_ message: String) {
    let line = "[\(Date())] \(message)\n"
    FileHandle.standardError.write("MatchaHelper: \(message)\n".data(using: .utf8) ?? Data())
    guard let data = line.data(using: .utf8) else { return }
    if let handle = FileHandle(forWritingAtPath: logFilePath) {
        handle.seekToEndOfFile()
        handle.write(data)
        try? handle.close()
    } else {
        try? data.write(to: URL(fileURLWithPath: logFilePath))
    }
}

// MARK: - JSON-RPC client over a Unix domain socket

final class DaemonClient {
    private let socketPath: String
    private var fd: Int32 = -1
    private var nextID: UInt64 = 0
    private let lock = NSLock()
    private var pending: [UInt64: (Any?) -> Void] = [:]
    private var readBuffer = Data()
    private let readQueue = DispatchQueue(label: "email.matcha.helper.read")

    var onEvent: ((String, [String: Any]) -> Void)?
    var onDisconnect: (() -> Void)?

    init(socketPath: String) {
        self.socketPath = socketPath
    }

    var isConnected: Bool {
        lock.lock(); defer { lock.unlock() }
        return fd >= 0
    }

    func connect() -> Bool {
        let s = socket(AF_UNIX, SOCK_STREAM, 0)
        if s < 0 { return false }

        var addr = sockaddr_un()
        addr.sun_family = sa_family_t(AF_UNIX)
        let pathCString = socketPath.utf8CString
        let capacity = MemoryLayout.size(ofValue: addr.sun_path)
        if pathCString.count > capacity {
            Darwin.close(s)
            return false
        }
        withUnsafeMutablePointer(to: &addr.sun_path) { rawPtr in
            rawPtr.withMemoryRebound(to: CChar.self, capacity: capacity) { dst in
                pathCString.withUnsafeBufferPointer { src in
                    dst.update(from: src.baseAddress!, count: src.count)
                }
            }
        }

        let size = socklen_t(MemoryLayout<sockaddr_un>.size)
        let result = withUnsafePointer(to: &addr) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                Darwin.connect(s, $0, size)
            }
        }
        if result != 0 {
            Darwin.close(s)
            return false
        }

        lock.lock()
        fd = s
        lock.unlock()
        startReadLoop()
        return true
    }

    func close() {
        lock.lock()
        let s = fd
        fd = -1
        let waiters = pending
        pending.removeAll()
        lock.unlock()
        if s >= 0 { Darwin.close(s) }
        for (_, cb) in waiters { cb(nil) }
    }

    // call sends a request and invokes completion with the raw result value
    // (a JSON object, array, or nil on error/disconnect). Pass a nil
    // completion for fire-and-forget calls (e.g. Subscribe).
    func call(_ method: String, params: [String: Any]? = nil, completion: ((Any?) -> Void)? = nil) {
        lock.lock()
        if fd < 0 {
            lock.unlock()
            completion?(nil)
            return
        }
        nextID += 1
        let id = nextID
        if let completion = completion {
            pending[id] = completion
        }
        lock.unlock()

        var message: [String: Any] = ["id": id, "method": method]
        if let params = params { message["params"] = params }

        guard var data = try? JSONSerialization.data(withJSONObject: message) else {
            removePending(id)?(nil)
            return
        }
        data.append(0x0A) // newline framing
        if !writeAll(data) {
            removePending(id)?(nil)
            close()
        }
    }

    private func removePending(_ id: UInt64) -> ((Any?) -> Void)? {
        lock.lock(); defer { lock.unlock() }
        return pending.removeValue(forKey: id)
    }

    private func writeAll(_ data: Data) -> Bool {
        lock.lock()
        let s = fd
        lock.unlock()
        if s < 0 { return false }
        return data.withUnsafeBytes { (raw: UnsafeRawBufferPointer) -> Bool in
            guard let base = raw.baseAddress else { return true }
            var total = 0
            let count = raw.count
            while total < count {
                let n = Darwin.write(s, base + total, count - total)
                if n <= 0 {
                    if n < 0 && errno == EINTR { continue }
                    return false
                }
                total += n
            }
            return true
        }
    }

    private func startReadLoop() {
        readQueue.async { [weak self] in
            guard let self = self else { return }
            var chunk = [UInt8](repeating: 0, count: 8192)
            while true {
                self.lock.lock()
                let s = self.fd
                self.lock.unlock()
                if s < 0 { break }

                let n = Darwin.read(s, &chunk, chunk.count)
                if n <= 0 {
                    if n < 0 && errno == EINTR { continue }
                    break
                }
                self.readBuffer.append(contentsOf: chunk[0..<n])
                self.drainBuffer()
            }
            self.close()
            DispatchQueue.main.async { self.onDisconnect?() }
        }
    }

    private func drainBuffer() {
        while let newlineIndex = readBuffer.firstIndex(of: 0x0A) {
            let lineData = readBuffer.subdata(in: readBuffer.startIndex..<newlineIndex)
            readBuffer.removeSubrange(readBuffer.startIndex...newlineIndex)
            guard !lineData.isEmpty,
                  let obj = try? JSONSerialization.jsonObject(with: lineData) as? [String: Any]
            else { continue }
            dispatchMessage(obj)
        }
    }

    private func dispatchMessage(_ obj: [String: Any]) {
        if let type = obj["type"] as? String {
            let data = obj["data"] as? [String: Any] ?? [:]
            DispatchQueue.main.async { [weak self] in self?.onEvent?(type, data) }
            return
        }
        guard let id = (obj["id"] as? NSNumber)?.uint64Value else { return }
        let result = obj["result"]
        if let cb = removePending(id) {
            DispatchQueue.main.async { cb(result) }
        }
    }
}

// MARK: - App

final class AppDelegate: NSObject, NSApplicationDelegate, UNUserNotificationCenterDelegate {
    private var statusItem: NSStatusItem!
    private var client: DaemonClient!
    private let presencePath: String

    private var accounts: [(id: String, email: String)] = []
    private var unreadByAccount: [String: Int] = [:]
    private var subscribed = Set<String>()
    private var reconnectScheduled = false
    private var refreshTimer: Timer?
    // Per-account fetch coalescing: at most one FetchFolders in flight per
    // account, with a single follow-up if more refreshes arrive meanwhile. This
    // stops a burst of events (NewMail + SyncComplete) from racing and firing
    // duplicate notifications for the same new message.
    private var fetchInFlight = Set<String>()
    private var refetchPending = Set<String>()

    private var socketPath: String {
        "\(NSHomeDirectory())/Library/Caches/matcha/daemon.sock"
    }

    override init() {
        presencePath = "\(NSHomeDirectory())/Library/Caches/matcha/helper.presence"
        super.init()
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        logLine("started")
        // Request notification permission on launch. This is the only way macOS
        // lets an app become "allowed" — there's no API to self-enable. On a
        // fresh install the user gets a one-tap "Allow" prompt (after which
        // banners are on by default); the Matcha icon comes from the bundle.
        let center = UNUserNotificationCenter.current()
        center.delegate = self
        center.requestAuthorization(options: [.alert, .sound, .badge]) { granted, error in
            if let error = error {
                logLine("notif auth error: \(error.localizedDescription)")
            } else {
                logLine("notif auth granted=\(granted)")
            }
        }
        writePresenceMarker()
        // Construct the client before building the status item: rebuildMenu()
        // reads client.isConnected.
        client = DaemonClient(socketPath: socketPath)
        client.onEvent = { [weak self] type, data in self?.handleEvent(type, data) }
        client.onDisconnect = { [weak self] in self?.handleDisconnect() }
        setupStatusItem()
        connectOrStartDaemon()

        refreshTimer = Timer.scheduledTimer(withTimeInterval: backstopRefreshInterval, repeats: true) { [weak self] _ in
            self?.refreshUnread()
        }
    }

    func applicationWillTerminate(_ notification: Notification) {
        try? FileManager.default.removeItem(atPath: presencePath)
    }

    // Presence marker: lets the daemon know a rich helper is handling
    // notifications. Best-effort; removed on quit.
    private func writePresenceMarker() {
        let dir = (presencePath as NSString).deletingLastPathComponent
        try? FileManager.default.createDirectory(atPath: dir, withIntermediateDirectories: true)
        FileManager.default.createFile(atPath: presencePath, contents: Data())
    }

    // MARK: Status item & menu

    private func setupStatusItem() {
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        if let button = statusItem.button {
            if let image = NSImage(contentsOfFile: iconPath) {
                image.size = NSSize(width: 18, height: 18)
                button.image = image
                button.imagePosition = .imageLeading
            } else {
                button.title = "🍵"
            }
        }
        rebuildMenu()
    }

    private var totalUnread: Int {
        unreadByAccount.values.reduce(0, +)
    }

    private func updateStatusTitle() {
        guard let button = statusItem.button else { return }
        let count = totalUnread
        button.title = count > 0 ? " \(count)" : ""
    }

    private func rebuildMenu() {
        let menu = NSMenu()

        let header = NSMenuItem(title: "Matcha", action: nil, keyEquivalent: "")
        header.isEnabled = false
        menu.addItem(header)
        menu.addItem(.separator())

        if !client.isConnected {
            let item = NSMenuItem(title: "Daemon not connected", action: nil, keyEquivalent: "")
            item.isEnabled = false
            menu.addItem(item)
        } else if accounts.isEmpty {
            let item = NSMenuItem(title: "No accounts configured", action: nil, keyEquivalent: "")
            item.isEnabled = false
            menu.addItem(item)
        } else {
            for account in accounts {
                let unread = unreadByAccount[account.id] ?? 0
                let title = unread > 0 ? "\(account.email) — \(unread) unread" : account.email
                let item = NSMenuItem(title: title, action: #selector(openMatcha), keyEquivalent: "")
                item.target = self
                menu.addItem(item)
            }
        }

        menu.addItem(.separator())

        let open = NSMenuItem(title: "Open Matcha", action: #selector(openMatcha), keyEquivalent: "o")
        open.target = self
        menu.addItem(open)

        let refresh = NSMenuItem(title: "Refresh", action: #selector(refreshClicked), keyEquivalent: "r")
        refresh.target = self
        menu.addItem(refresh)

        menu.addItem(.separator())

        let quit = NSMenuItem(title: "Quit", action: #selector(quitClicked), keyEquivalent: "q")
        quit.target = self
        menu.addItem(quit)

        statusItem.menu = menu
    }

    private func refreshUI() {
        updateStatusTitle()
        rebuildMenu()
    }

    // MARK: Connection

    private func connectOrStartDaemon() {
        if client.connect() {
            onConnected()
            return
        }
        // Daemon likely not running — start it, then retry with backoff.
        startDaemon()
        scheduleReconnect()
    }

    private func startDaemon() {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: matchaPath)
        process.arguments = ["daemon", "start"]
        do {
            try process.run()
        } catch {
            logLine("failed to start daemon: \(error.localizedDescription)")
        }
    }

    private func scheduleReconnect() {
        if reconnectScheduled { return }
        reconnectScheduled = true
        DispatchQueue.main.asyncAfter(deadline: .now() + 2.0) { [weak self] in
            guard let self = self else { return }
            self.reconnectScheduled = false
            if self.client.connect() {
                self.onConnected()
            } else {
                self.scheduleReconnect()
            }
        }
    }

    private func onConnected() {
        subscribed.removeAll()
        loadAccounts()
        refreshUI()
    }

    private func handleDisconnect() {
        accounts = []
        unreadByAccount = [:]
        subscribed.removeAll()
        refreshUI()
        scheduleReconnect()
    }

    // MARK: Data

    private func loadAccounts() {
        client.call("GetAccounts") { [weak self] result in
            guard let self = self else { return }
            let array = (result as? [[String: Any]]) ?? []
            self.accounts = array.compactMap { item in
                guard let id = item["id"] as? String,
                      let email = item["email"] as? String else { return nil }
                return (id: id, email: email)
            }
            for account in self.accounts {
                self.subscribeInbox(account.id)
            }
            self.refreshUnread()
            self.refreshUI()
        }
    }

    private func subscribeInbox(_ accountID: String) {
        if subscribed.contains(accountID) { return }
        subscribed.insert(accountID)
        client.call("Subscribe", params: ["account_id": accountID, "folder": "INBOX"])
    }

    private func refreshUnread() {
        for account in accounts {
            fetchUnread(account.id)
        }
    }

    private func fetchUnread(_ accountID: String) {
        // Coalesce: if a fetch is already running for this account, just mark a
        // follow-up and return, so concurrent triggers don't double-count.
        if fetchInFlight.contains(accountID) {
            refetchPending.insert(accountID)
            return
        }
        fetchInFlight.insert(accountID)
        client.call("FetchFolders", params: ["account_id": accountID]) { [weak self] result in
            guard let self = self else { return }
            self.fetchInFlight.remove(accountID)
            let folders = (result as? [[String: Any]]) ?? []
            // Count INBOX unread only. Summing every folder would fold in
            // Trash/Junk/Archive (e.g. hundreds of "unread" deleted
            // messages), which isn't what the badge should reflect — and it
            // matches what the daemon watches and notifies on.
            var total = 0
            for folder in folders {
                guard let name = folder["Name"] as? String,
                      name.uppercased() == "INBOX" else { continue }
                if let unread = (folder["Unread"] as? NSNumber)?.intValue {
                    total += unread
                }
            }
            // Notify when INBOX unread rises. This is more reliable than the
            // daemon's NewMail event (which depends on IMAP IDLE pushing in
            // time): any path that detects new mail — IDLE, a sync, or the
            // backstop poll — raises the count and triggers the banner. A nil
            // previous value means this is the first read (or a reconnect),
            // so we don't notify for the initial state.
            let previous = self.unreadByAccount[accountID]
            self.unreadByAccount[accountID] = total
            if previous != total {
                logLine("unread INBOX for \(self.emailForAccount(accountID)) = \(total) (total \(self.totalUnread))")
            }
            if let previous = previous, total > previous {
                self.notifyNewMail(accountID: accountID, newCount: total - previous)
            }
            self.refreshUI()
            // Run a single coalesced follow-up if more triggers arrived while
            // this fetch was in flight.
            if self.refetchPending.remove(accountID) != nil {
                self.fetchUnread(accountID)
            }
        }
    }

    // MARK: Events

    private func handleEvent(_ type: String, _ data: [String: Any]) {
        // Every event that could mean new mail just re-reads the unread count;
        // refreshUnread() posts the notification when the count rises. This keeps
        // a single, reliable notification trigger instead of depending on NewMail.
        switch type {
        case "NewMail", "EmailsUpdated", "SyncComplete":
            refreshUnread()
        default:
            break
        }
    }

    private func emailForAccount(_ accountID: String) -> String {
        accounts.first(where: { $0.id == accountID })?.email ?? accountID
    }

    private func notifyNewMail(accountID: String, newCount: Int) {
        let account = emailForAccount(accountID)
        let content = UNMutableNotificationContent()
        content.title = newCount > 1 ? "\(newCount) new messages" : "New mail"
        content.body = account.isEmpty ? "You have new mail" : "New mail for \(account)"
        content.sound = .default
        // The Matcha app icon (the bundle icon) is shown on the banner by macOS
        // automatically, so no attachment is needed.
        let request = UNNotificationRequest(identifier: UUID().uuidString, content: content, trigger: nil)
        UNUserNotificationCenter.current().add(request) { error in
            if let error = error {
                logLine("notif add error for \(account): \(error.localizedDescription)")
            } else {
                logLine("posted notification for \(account)")
            }
        }
    }

    // Show the banner even if the agent happens to be frontmost.
    func userNotificationCenter(_ center: UNUserNotificationCenter,
                                willPresent notification: UNNotification,
                                withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void) {
        completionHandler([.banner, .sound])
    }

    // Open Matcha when the user clicks the banner.
    func userNotificationCenter(_ center: UNUserNotificationCenter,
                                didReceive response: UNNotificationResponse,
                                withCompletionHandler completionHandler: @escaping () -> Void) {
        launchMatcha()
        completionHandler()
    }

    // MARK: Actions

    @objc private func openMatcha() {
        launchMatcha()
    }

    @objc private func refreshClicked() {
        if client.isConnected {
            refreshUnread()
        } else {
            connectOrStartDaemon()
        }
    }

    @objc private func quitClicked() {
        NSApp.terminate(nil)
    }

    // launchMatcha opens matcha in the user's default terminal via a temporary
    // .command file (same mechanism as cli/macos_handler.swift).
    private func launchMatcha() {
        let tempDir = NSTemporaryDirectory()
        let fileName = "matcha-open-\(UUID().uuidString).command"
        let fileURL = URL(fileURLWithPath: tempDir).appendingPathComponent(fileName)
        let script = """
        #!/bin/bash
        '\(matchaPath)'
        rm -- "$0"
        exit
        """
        do {
            try script.write(to: fileURL, atomically: true, encoding: .utf8)
            try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: fileURL.path)
            NSWorkspace.shared.open(fileURL)
        } catch {
            logLine("failed to launch matcha: \(error.localizedDescription)")
        }
    }
}

let app = NSApplication.shared
app.setActivationPolicy(.accessory)
let delegate = AppDelegate()
app.delegate = delegate
app.run()
