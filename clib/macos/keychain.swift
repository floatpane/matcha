import Foundation
import Security

// Compilation: swiftc keychain.swift -o keychain
// Usage: ./keychain <get|set|delete> <service> <account> [password]

enum KeychainError: Error {
    case unhandledError(status: OSStatus)
}

func setPassword(service: String, account: String, password: Data) throws {
    let query: [String: Any] = [
        kSecClass as String: kSecClassGenericPassword,
        kSecAttrService as String: service,
        kSecAttrAccount as String: account,
        kSecValueData as String: password,
        kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlock
    ]

    let status = SecItemAdd(query as CFDictionary, nil)
    
    if status == errSecDuplicateItem {
        let updateQuery: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account
        ]
        let attributesToUpdate: [String: Any] = [
            kSecValueData as String: password
        ]
        let updateStatus = SecItemUpdate(updateQuery as CFDictionary, attributesToUpdate as CFDictionary)
        if updateStatus != errSecSuccess {
            throw KeychainError.unhandledError(status: updateStatus)
        }
    } else if status != errSecSuccess {
        throw KeychainError.unhandledError(status: status)
    }
}

func getPassword(service: String, account: String) throws -> Data? {
    let query: [String: Any] = [
        kSecClass as String: kSecClassGenericPassword,
        kSecAttrService as String: service,
        kSecAttrAccount as String: account,
        kSecReturnData as String: kCFBooleanTrue!,
        kSecMatchLimit as String: kSecMatchLimitOne
    ]

    var dataTypeRef: AnyObject?
    let status = SecItemCopyMatching(query as CFDictionary, &dataTypeRef)

    if status == errSecItemNotFound {
        return nil
    } else if status != errSecSuccess {
        throw KeychainError.unhandledError(status: status)
    }

    return dataTypeRef as? Data
}

func deletePassword(service: String, account: String) throws {
    let query: [String: Any] = [
        kSecClass as String: kSecClassGenericPassword,
        kSecAttrService as String: service,
        kSecAttrAccount as String: account
    ]

    let status = SecItemDelete(query as CFDictionary)
    if status != errSecSuccess && status != errSecItemNotFound {
        throw KeychainError.unhandledError(status: status)
    }
}

let args = ProcessInfo.processInfo.arguments
guard args.count >= 4 else {
    print("Usage: ./keychain <get|set|delete> <service> <account> [password]")
    exit(1)
}

let action = args[1]
let service = args[2]
let account = args[3]

do {
    switch action {
    case "set":
        guard args.count > 4 else { exit(1) }
        let password = args[4].data(using: .utf8)!
        try setPassword(service: service, account: account, password: password)
        print("Success")
    case "get":
        if let data = try getPassword(service: service, account: account),
           let password = String(data: data, encoding: .utf8) {
            print(password)
        }
    case "delete":
        try deletePassword(service: service, account: account)
        print("Success")
    default:
        exit(1)
    }
} catch {
    print("Error: \(error)")
    exit(1)
}
