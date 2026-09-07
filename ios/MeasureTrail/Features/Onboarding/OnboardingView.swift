import SwiftUI

struct OnboardingView: View {
    @Environment(AppModel.self) private var appModel
    @State private var unit = WeightUnit.defaultUnit

    var body: some View {
        VStack(alignment: .leading, spacing: 28) {
            Spacer()
            Image(systemName: "point.3.connected.trianglepath.dotted")
                .font(.system(size: 54, weight: .medium))
                .foregroundStyle(MeasureTrailStyle.ink)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 10) {
                Text("欢迎来到量迹")
                    .font(.largeTitle.weight(.bold))
                Text("先选择你习惯的体重单位。之后随时可以在“我的”中调整。")
                    .font(.body)
                    .foregroundStyle(.secondary)
            }
            VStack(alignment: .leading, spacing: 14) {
                Text("体重单位")
                    .font(.headline)
                Picker("体重单位", selection: $unit) {
                    Text("公斤（kg）").tag("kg")
                    Text("斤").tag("jin")
                }
                .pickerStyle(.segmented)
                .accessibilityHint("这会影响体重数值的显示方式")
            }
            .measureTrailActionSurface()
            Spacer()
            Button("继续") {
                WeightUnit.save(unit)
                appModel.completeOnboarding()
            }
            .buttonStyle(.borderedProminent)
            .tint(MeasureTrailStyle.accent)
            .controlSize(.large)
            .frame(maxWidth: .infinity)
        }
        .padding()
    }
}
