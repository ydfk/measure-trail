import SwiftUI

struct ProfileSettingsView: View {
    @State private var height = ""
    @State private var targetWeight = ""
    @State private var preferredUnit = "kg"
    @State private var timezone = TimeZone.current.identifier
    @State private var isLoading = true
    @State private var isSaving = false
    @State private var message: String?

    var body: some View {
        Form {
            Section("显示单位") {
                Picker("体重单位", selection: $preferredUnit) {
                    Text("公斤（kg）").tag("kg")
                    Text("斤").tag("jin")
                }
                .pickerStyle(.segmented)
                .onChange(of: preferredUnit) { previous, current in
                    guard let optionalGrams = weightInGrams(targetWeight, unit: previous), let grams = optionalGrams else { return }
                    targetWeight = displayWeight(grams, unit: current)
                }
            }
            Section("个人目标") {
                TextField("身高（cm，可选）", text: $height).keyboardType(.decimalPad)
                TextField("目标体重（可选）", text: $targetWeight).keyboardType(.decimalPad)
                TextField("时区", text: $timezone).textInputAutocapitalization(.never).autocorrectionDisabled()
            }
            Section {
                Text("身高只用于在概览中计算 BMI；目标体重只用于显示进度。它们不是医疗建议。")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            if let message {
                Section { Label(message, systemImage: "exclamationmark.circle").foregroundStyle(.secondary) }
            }
        }
        .navigationTitle("资料与目标")
        .overlay { if isLoading { ProgressView("正在读取资料") } }
        .toolbar { ToolbarItem(placement: .confirmationAction) { Button(isSaving ? "保存中" : "保存") { Task { await save() } }.disabled(isLoading || isSaving) } }
        .task { await load() }
    }

    private func load() async {
        guard let session = TokenStore().session() else { message = "本地登录状态已失效。"; isLoading = false; return }
        do {
            apply(try await APIClient().profile(accessToken: session.accessToken))
        } catch {
            message = error.localizedDescription
        }
        isLoading = false
    }

    private func save() async {
        guard let session = TokenStore().session() else { message = "本地登录状态已失效。"; return }
        guard let heightMM = heightInMillimeters(height), let targetWeightG = weightInGrams(targetWeight, unit: preferredUnit), !timezone.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            message = "请检查身高、目标体重和时区。"
            return
        }
        isSaving = true
        do {
            apply(try await APIClient().updateProfile(heightMM: heightMM, targetWeightG: targetWeightG, preferredUnit: preferredUnit, timezone: timezone.trimmingCharacters(in: .whitespacesAndNewlines), accessToken: session.accessToken))
            message = "资料已保存。"
        } catch {
            message = error.localizedDescription
        }
        isSaving = false
    }

    private func apply(_ profile: APIClient.ProfileResponse) {
        WeightUnit.save(profile.preferredUnit)
        preferredUnit = profile.preferredUnit
        height = profile.heightMM.map { displayHeight($0) } ?? ""
        targetWeight = profile.targetWeightG.map { displayWeight($0, unit: profile.preferredUnit) } ?? ""
        timezone = profile.timezone
    }

    private func heightInMillimeters(_ value: String) -> Int?? {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return .some(nil) }
        guard let centimeters = Decimal(string: trimmed.replacingOccurrences(of: ",", with: ".")) else { return nil }
        let millimeters = rounded(centimeters * 10)
        return (500...3_000).contains(millimeters) ? .some(millimeters) : nil
    }

    private func weightInGrams(_ value: String, unit: String) -> Int?? {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return .some(nil) }
        guard let weight = Decimal(string: trimmed.replacingOccurrences(of: ",", with: ".")) else { return nil }
        let grams = rounded(weight * (unit == "jin" ? 500 : 1_000))
        return (10_000...500_000).contains(grams) ? .some(grams) : nil
    }

    private func rounded(_ value: Decimal) -> Int {
        NSDecimalNumber(decimal: value).rounding(accordingToBehavior: NSDecimalNumberHandler(roundingMode: .plain, scale: 0, raiseOnExactness: false, raiseOnOverflow: false, raiseOnUnderflow: false, raiseOnDivideByZero: false)).intValue
    }

    private func displayHeight(_ millimeters: Int) -> String { Double(millimeters).formatted(.number.precision(.fractionLength(1))) }
    private func displayWeight(_ grams: Int, unit: String) -> String { (Double(grams) / (unit == "jin" ? 500 : 1_000)).formatted(.number.precision(.fractionLength(1))) }
}
