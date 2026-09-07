import SwiftUI
import UIKit

enum MeasureTrailStyle {
    static let ink = Color(red: 18 / 255, green: 58 / 255, blue: 112 / 255)
    static let blue = Color(red: 40 / 255, green: 120 / 255, blue: 207 / 255)
    static let highlight = Color(red: 241 / 255, green: 128 / 255, blue: 92 / 255)
    static let accent = Color(uiColor: UIColor { traits in
        traits.userInterfaceStyle == .dark
            ? UIColor(red: 100 / 255, green: 174 / 255, blue: 1, alpha: 1)
            : UIColor(red: 18 / 255, green: 58 / 255, blue: 112 / 255, alpha: 1)
    })
}

extension View {
    func measureTrailActionSurface() -> some View {
        padding(20)
            .background(.thinMaterial, in: RoundedRectangle(cornerRadius: 28, style: .continuous))
    }
}
