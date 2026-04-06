import SwiftUI

struct CompletionBannerView: View {
    @EnvironmentObject var model: AppModel
    @State private var autoDismissTask: Task<Void, Never>?

    var body: some View {
        if model.showingCompletionBanner, let clean = model.lastCleanResult {
            HStack(spacing: 12) {
                Image(systemName: "checkmark.circle.fill")
                    .font(.title2)
                    .foregroundStyle(.green)

                VStack(alignment: .leading, spacing: 2) {
                    Text("Reclaimed: \(formatBytes(clean.cleanedSize))")
                        .font(.headline)
                    Text("\(clean.cleanedCount) items moved to Trash")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }

                Spacer()

                HStack(spacing: 8) {
                    Button("View Log") {
                        model.openAuditLog()
                    }
                    .buttonStyle(.bordered)
                    .controlSize(.small)

                    Button {
                        model.undoLastClean()
                    } label: {
                        Label("Undo", systemImage: "arrow.uturn.backward")
                    }
                    .buttonStyle(.bordered)
                    .controlSize(.small)
                    .disabled(model.activity != .idle)

                    Button("Dismiss") {
                        withAnimation { model.showingCompletionBanner = false }
                    }
                    .buttonStyle(.plain)
                    .foregroundStyle(.secondary)
                    .controlSize(.small)
                }
            }
            .padding(14)
            .background(.regularMaterial)
            .clipShape(RoundedRectangle(cornerRadius: OCLayout.cornerRadius))
            .shadow(color: .black.opacity(0.15), radius: 8, y: 4)
            .padding(.horizontal, 20)
            .padding(.bottom, 16)
            .transition(.move(edge: .bottom).combined(with: .opacity))
            .onAppear {
                autoDismissTask?.cancel()
                autoDismissTask = Task {
                    try? await Task.sleep(for: .seconds(30))
                    if !Task.isCancelled {
                        await MainActor.run {
                            withAnimation { model.showingCompletionBanner = false }
                        }
                    }
                }
            }
            .onDisappear {
                autoDismissTask?.cancel()
            }
        }
    }
}
