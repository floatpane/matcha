import Foundation
import CoreSpotlight
import MobileCoreServices

// Compilation: swiftc spotlight.swift -o spotlight
// Usage: ./spotlight <json_of_emails>

struct EmailToIndex: Codable {
    let id: String
    let subject: String
    let sender: String
    let body: String
    let date: Date
}

func indexEmails(jsonString: String) {
    guard let data = jsonString.data(using: .utf8),
          let emails = try? JSONDecoder().decode([EmailToIndex].self, from: data) else {
        print("Error: Invalid JSON")
        exit(1)
    }
    
    var searchableItems: [CSSearchableItem] = []
    
    for email in emails {
        let attributeSet = CSSearchableItemAttributeSet(itemContentType: kUTTypeEmailMessage as String)
        attributeSet.title = email.subject
        attributeSet.contentDescription = email.body
        attributeSet.authorNames = [email.sender]
        attributeSet.contentCreationDate = email.date
        
        let item = CSSearchableItem(
            uniqueIdentifier: email.id,
            domainIdentifier: "com.floatpane.matcha.emails",
            attributeSet: attributeSet
        )
        searchableItems.append(item)
    }
    
    CSSearchableIndex.default().indexSearchableItems(searchableItems) { error in
        if let error = error {
            print("Error indexing items: \(error.localizedDescription)")
            exit(1)
        } else {
            print("Successfully indexed \(searchableItems.count) emails")
            exit(0)
        }
    }
}

let args = ProcessInfo.processInfo.arguments
if args.count > 1 {
    indexEmails(jsonString: args[1])
} else {
    let input = FileHandle.standardInput.readDataToEndOfFile()
    if let text = String(data: input, encoding: .utf8) {
        indexEmails(jsonString: text)
    }
}

RunLoop.main.run()
