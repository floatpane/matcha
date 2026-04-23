import Foundation
import Contacts

// Compilation: swiftc contacts.swift -o contacts
// Usage: ./contacts [searchQuery]

struct ContactEntry: Codable {
    let id: String
    let name: String
    let emails: [String]
    let thumbnail: String? // Base64 encoded image data
}

func fetchContacts(query: String?) {
    let store = CNContactStore()
    
    store.requestAccess(for: .contacts) { (granted, error) in
        guard granted else {
            print("[]")
            exit(0)
        }
        
        let keys = [
            CNContactGivenNameKey as CNKeyDescriptor,
            CNContactFamilyNameKey as CNKeyDescriptor,
            CNContactEmailAddressesKey as CNKeyDescriptor,
            CNContactThumbnailImageDataKey as CNKeyDescriptor,
            CNContactIdentifierKey as CNKeyDescriptor
        ]
        
        var results: [ContactEntry] = []
        let fetchRequest = CNContactFetchRequest(keysToFetch: keys)
        
        // If a query is provided, we use a predicate to filter contacts by name
        if let query = query, !query.isEmpty {
            fetchRequest.predicate = CNContact.predicateForContacts(matchingName: query)
        }

        do {
            try store.enumerateContacts(with: fetchRequest) { (contact, stop) in
                let fullName = [contact.givenName, contact.familyName]
                    .filter { !$0.isEmpty }
                    .joined(separator: " ")
                
                let emails = contact.emailAddresses.map { $0.value as String }
                
                // Only include contacts that actually have an email address
                if !emails.isEmpty {
                    var thumbnailBase64: String? = nil
                    if let imageData = contact.thumbnailImageData {
                        thumbnailBase64 = imageData.base64EncodedString()
                    }
                    
                    results.append(ContactEntry(
                        id: contact.identifier,
                        name: fullName,
                        emails: emails,
                        thumbnail: thumbnailBase64
                    ))
                }
            }
            
            let encoder = JSONEncoder()
            if let data = try? encoder.encode(results),
               let jsonString = String(data: data, encoding: .utf8) {
                print(jsonString)
            }
        } catch {
            print("[]")
        }
        exit(0)
    }
}

let args = ProcessInfo.processInfo.arguments
let query = args.count > 1 ? args[1] : nil

fetchContacts(query: query)
RunLoop.main.run()
