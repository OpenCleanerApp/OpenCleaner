import SwiftUI

struct OnboardingView: View {
    @EnvironmentObject var model: AppModel
    @State private var step: OnboardingStep = .welcome
    @State private var permissionTimer: Timer?

    enum OnboardingStep {
        case welcome
        case permission
        case waiting
        case ready
    }

    var body: some View {
        VStack(spacing: 0) {
            Spacer()

            switch step {
            case .welcome:
                welcomeStep
            case .permission:
                permissionStep
            case .waiting:
                waitingStep
            case .ready:
                readyStep
            }

            Spacer()
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .padding(40)
        .onDisappear {
            permissionTimer?.invalidate()
        }
    }

    // MARK: - Steps

    private var welcomeStep: some View {
        VStack(spacing: 20) {
            Image(systemName: "sparkles")
                .font(.system(size: 64))
                .foregroundStyle(.tint)

            Text("Welcome to OpenCleaner")
                .font(.largeTitle.bold())

            Text("A developer-focused macOS disk cleaner that helps you reclaim space from caches, build artifacts, and system junk.")
                .font(.body)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
                .frame(maxWidth: 420)

            Button("Get Started") {
                withAnimation { step = .permission }
            }
            .buttonStyle(.borderedProminent)
            .controlSize(.large)
        }
    }

    private var permissionStep: some View {
        VStack(spacing: 20) {
            Image(systemName: "lock.shield")
                .font(.system(size: 56))
                .foregroundStyle(.orange)

            Text("Full Disk Access Required")
                .font(.title.bold())

            Text("OpenCleaner needs Full Disk Access to scan system caches, application support folders, and developer build artifacts across your Mac.")
                .font(.body)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
                .frame(maxWidth: 420)

            VStack(spacing: 12) {
                Button("Grant Access") {
                    PermissionService.openFullDiskAccessSettings()
                    withAnimation { step = .waiting }
                    startPolling()
                }
                .buttonStyle(.borderedProminent)
                .controlSize(.large)

                Button("Skip for Now") {
                    completeOnboarding()
                }
                .buttonStyle(.plain)
                .foregroundStyle(.secondary)
                .font(.callout)
            }
        }
    }

    private var waitingStep: some View {
        VStack(spacing: 20) {
            ProgressView()
                .scaleEffect(1.5)
                .padding(.bottom, 8)

            Text("Waiting for Permission…")
                .font(.title2.bold())

            Text("Toggle OpenCleaner **on** in System Settings → Privacy & Security → Full Disk Access, then return here.")
                .font(.body)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
                .frame(maxWidth: 420)

            HStack(spacing: 16) {
                Button("Open Settings Again") {
                    PermissionService.openFullDiskAccessSettings()
                }
                .buttonStyle(.bordered)

                Button("Skip for Now") {
                    permissionTimer?.invalidate()
                    completeOnboarding()
                }
                .buttonStyle(.plain)
                .foregroundStyle(.secondary)
                .font(.callout)
            }
        }
    }

    private var readyStep: some View {
        VStack(spacing: 20) {
            Image(systemName: "checkmark.circle.fill")
                .font(.system(size: 64))
                .foregroundStyle(.green)

            Text("You're All Set!")
                .font(.largeTitle.bold())

            Text("Full Disk Access granted. OpenCleaner can now scan all system caches and build artifacts.")
                .font(.body)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
                .frame(maxWidth: 420)

            Button("Start First Scan") {
                completeOnboarding()
                model.scan()
            }
            .buttonStyle(.borderedProminent)
            .controlSize(.large)
        }
    }

    // MARK: - Helpers

    private func startPolling() {
        permissionTimer?.invalidate()
        permissionTimer = Timer.scheduledTimer(withTimeInterval: 2.0, repeats: true) { _ in
            Task { @MainActor in
                model.checkPermission()
                if model.hasFullDiskAccess {
                    permissionTimer?.invalidate()
                    withAnimation { step = .ready }
                }
            }
        }
    }

    private func completeOnboarding() {
        model.hasCompletedOnboarding = true
    }
}
