import Cocoa

// Compilation: swiftc spellcheck.swift -o spellcheck
// Usage: ./spellcheck "Your text with typos here"

struct Misspelling: Codable {
    let word: String
    let suggestions: [String]
    let range: [Int] // [location, length]
}

func checkSpelling(text: String) {
    let spellChecker = NSSpellChecker.shared
    let range = NSRange(location: 0, length: text.utf16.count)
    var misspellings: [Misspelling] = []
    
    var offset = 0
    while offset < range.length {
        let currentRange = NSRange(location: offset, length: range.length - offset)
        let misspelledRange = spellChecker.checkSpelling(of: text, startingAt: currentRange.location)
        
        if misspelledRange.location == NSNotFound || misspelledRange.length == 0 {
            break
        }
        
        let word = (text as NSString).substring(with: misspelledRange)
        let suggestions = spellChecker.guesses(forWordRange: misspelledRange, in: text, language: nil, inSpellDocumentWithTag: 0) ?? []
        
        misspellings.append(Misspelling(
            word: word,
            suggestions: Array(suggestions.prefix(3)), // Top 3 suggestions
            range: [misspelledRange.location, misspelledRange.length]
        ))
        
        offset = misspelledRange.location + misspelledRange.length
    }
    
    let encoder = JSONEncoder()
    if let data = try? encoder.encode(misspellings),
       let jsonString = String(data: data, encoding: .utf8) {
        print(jsonString)
    }
}

let args = ProcessInfo.processInfo.arguments
if args.count > 1 {
    checkSpelling(text: args[1])
} else {
    // Read from stdin if no arg provided
    let input = FileHandle.standardInput.readDataToEndOfFile()
    if let text = String(data: input, encoding: .utf8) {
        checkSpelling(text: text)
    }
}
