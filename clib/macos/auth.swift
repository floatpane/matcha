import Foundation
import LocalAuthentication

// Compilation: swiftc auth.swift -o auth
// Usage: ./auth <reason>

enum AuthResult: String, Codable {
    case success
    case failure
    case notAvailable
    case userCancel
    case fallback
}

struct Response: Codable {
    let status: AuthResult
    let message: String?
}

func authenticate(reason: String) {
    let context = LAContext()
    var error: NSError?

    // Check if biometric authentication is available on the device.
    if context.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: &error) {
        
        context.evaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, localizedReason: reason) { success, authenticationError in
            
            DispatchQueue.main.async {
                if success {
                    sendResponse(status: .success, message: "Successfully authenticated")
                } else {
                    if let error = authenticationError as? LAError {
                        switch error.code {
                        case .userCancel:
                            sendResponse(status: .userCancel, message: "User cancelled")
                        case .userFallback:
                            sendResponse(status: .fallback, message: "User chose fallback")
                        case .biometryNotEnrolled:
                            sendResponse(status: .notAvailable, message: "Biometry not enrolled")
                        case .biometryLockout:
                            sendResponse(status: .failure, message: "Biometry lockout")
                        default:
                            sendResponse(status: .failure, message: error.localizedDescription)
                        }
                    } else {
                        sendResponse(status: .failure, message: "Unknown error")
                    }
                }
            }
        }
    } else {
        // Biometry is not available on this device
        let message = error?.localizedDescription ?? "Biometry not available"
        sendResponse(status: .notAvailable, message: message)
    }
}

func sendResponse(status: AuthResult, message: String?) {
    let response = Response(status: status, message: message)
    if let data = try? JSONEncoder().encode(response),
       let jsonString = String(data: data, encoding: .utf8) {
        print(jsonString)
    }
    exit(0)
}

let args = ProcessInfo.processInfo.arguments
let reason = args.count > 1 ? args[1] : "Authenticate to unlock Matcha"

authenticate(reason: reason)

// Keep the process alive for the async evaluation
RunLoop.main.run()
