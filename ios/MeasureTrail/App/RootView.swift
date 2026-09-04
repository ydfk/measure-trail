import SwiftUI

struct RootView: View {
    @Environment(AppModel.self) private var appModel

    var body: some View {
        Group {
            switch appModel.state {
            case .loading:
                ProgressView("正在准备量迹")
            case .onboarding:
                OnboardingView()
            case .signedOut:
                AuthenticationView()
            case .signedIn:
                MainTabView()
            }
        }
    }
}
