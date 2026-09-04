import Foundation

struct MeasurementInput {
    let weightG: Int
    let waistMM: Int?
    let note: String

    static func make(weightText: String, waistText: String, note: String) throws -> MeasurementInput {
        guard let weightG = WeightUnit.grams(from: weightText) else { throw ValidationError.invalidWeight }
        guard (10_000...500_000).contains(weightG) else { throw ValidationError.invalidWeight }
        let waistMM: Int?
        if waistText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty { waistMM = nil }
        else {
            guard let waist = Decimal(string: waistText.replacingOccurrences(of: ",", with: ".")), waist > 0 else { throw ValidationError.invalidWaist }
            let value = NSDecimalNumber(decimal: waist * 10).rounding(accordingToBehavior: NSDecimalNumberHandler(roundingMode: .plain, scale: 0, raiseOnExactness: false, raiseOnOverflow: false, raiseOnUnderflow: false, raiseOnDivideByZero: false)).intValue
            guard (100...3_000).contains(value) else { throw ValidationError.invalidWaist }
            waistMM = value
        }
        guard note.count <= 500 else { throw ValidationError.noteTooLong }
        return MeasurementInput(weightG: weightG, waistMM: waistMM, note: note.trimmingCharacters(in: .whitespacesAndNewlines))
    }
}

enum ValidationError: LocalizedError { case invalidWeight, invalidWaist, noteTooLong
    var errorDescription: String? { switch self { case .invalidWeight: "请输入合理范围内的体重。"; case .invalidWaist: "请输入 10–300 cm 范围内的腰围。"; case .noteTooLong: "备注最多 500 个字符。" } }
}
