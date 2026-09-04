import Foundation
import Observation

@Observable
@MainActor
final class AppModel {
    enum State { case loading, onboarding, signedOut, signedIn }

    private static let completedOnboardingKey = "completedOnboarding"

    private(set) var state: State = .loading

    init() {
        if ProcessInfo.processInfo.arguments.contains("-uiTestingResetOnboarding") {
            UserDefaults.standard.removeObject(forKey: Self.completedOnboardingKey)
            UserDefaults.standard.removeObject(forKey: AppConfiguration.apiBaseURLKey)
            TokenStore().clear()
        }
        if !UserDefaults.standard.bool(forKey: Self.completedOnboardingKey) {
            state = .onboarding
        } else {
            state = TokenStore().hasSession ? .signedIn : .signedOut
        }
    }

    func completeOnboarding() {
        UserDefaults.standard.set(true, forKey: Self.completedOnboardingKey)
        state = TokenStore().hasSession ? .signedIn : .signedOut
    }

    func didAuthenticate() { state = .signedIn }
    func signOut() { TokenStore().clear(); state = .signedOut }
}
