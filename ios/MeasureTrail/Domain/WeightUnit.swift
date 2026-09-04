import Foundation

enum WeightUnit {
    private static let preferenceKey = "preferredWeightUnit"

    static var defaultUnit: String {
        Locale.current.language.languageCode?.identifier == "zh" ? "jin" : "kg"
    }

    static var current: String {
        guard let stored = UserDefaults.standard.string(forKey: preferenceKey) else { return defaultUnit }
        return stored == "jin" ? "jin" : "kg"
    }

    static var label: String { current == "jin" ? "斤" : "kg" }

    static func save(_ unit: String) {
        UserDefaults.standard.set(unit == "jin" ? "jin" : "kg", forKey: preferenceKey)
    }

    static func value(fromGrams grams: Int) -> Double {
        Double(grams) / (current == "jin" ? 500 : 1_000)
    }

    static func display(_ grams: Int) -> String {
        "\(value(fromGrams: grams).formatted(.number.precision(.fractionLength(1)))) \(label)"
    }

    static func inputText(_ grams: Int) -> String {
        value(fromGrams: grams).formatted(.number.precision(.fractionLength(1)))
    }

    static func grams(from input: String) -> Int? {
        guard let value = Decimal(string: input.replacingOccurrences(of: ",", with: ".")), value > 0 else { return nil }
        return rounded(value * (current == "jin" ? 500 : 1_000))
    }

    private static func rounded(_ value: Decimal) -> Int {
        NSDecimalNumber(decimal: value).rounding(accordingToBehavior: NSDecimalNumberHandler(roundingMode: .plain, scale: 0, raiseOnExactness: false, raiseOnOverflow: false, raiseOnUnderflow: false, raiseOnDivideByZero: false)).intValue
    }
}
