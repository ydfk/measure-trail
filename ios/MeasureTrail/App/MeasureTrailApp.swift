import SwiftUI

@main
struct MeasureTrailApp: App {
    @State private var appModel = AppModel()

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(appModel)
        }
        .modelContainer(for: [CachedMeasurement.self, PendingMutation.self, SyncConflict.self, SyncCheckpoint.self])
    }
}
