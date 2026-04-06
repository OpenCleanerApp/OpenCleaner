import AppKit
import OpenCleanerClient
import SwiftUI

enum OCLayout {
    static let cornerRadius: CGFloat = 12
    static let pagePadding: CGFloat = 16
    static let labelColumnWidth: CGFloat = 88
}

func copyToPasteboard(_ string: String) {
    NSPasteboard.general.clearContents()
    NSPasteboard.general.setString(string, forType: .string)
}

struct OCCard<Content: View>: View {
    var title: String
    var systemImage: String? = nil
    @ViewBuilder var content: () -> Content

    var body: some View {
        GroupBox {
            content()
                .frame(maxWidth: .infinity, alignment: .leading)
        } label: {
            if let systemImage {
                Label(title, systemImage: systemImage)
                    .font(.headline)
            } else {
                Text(title)
                    .font(.headline)
            }
        }
    }
}

struct MetricView: View {
    var title: String
    var value: String

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(title)
                .font(.caption)
                .foregroundStyle(.secondary)
            Text(value)
                .font(.headline)
                .monospacedDigit()
        }
    }
}

struct DaemonStatusHeader<Trailing: View>: View {
    var statusColor: Color
    var title: String
    var subtitle: String
    @ViewBuilder var trailing: () -> Trailing

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 10) {
            Circle()
                .fill(statusColor)
                .frame(width: 9, height: 9)

            VStack(alignment: .leading, spacing: 2) {
                Text(title)
                    .font(.headline)

                Text(subtitle)
                    .font(.caption.monospaced())
                    .foregroundStyle(.secondary)
                    .textSelection(.enabled)
                    .lineLimit(1)
                    .truncationMode(.middle)
            }

            Spacer(minLength: 0)

            trailing()
        }
    }
}

extension DaemonStatusHeader where Trailing == EmptyView {
    init(statusColor: Color, title: String, subtitle: String) {
        self.statusColor = statusColor
        self.title = title
        self.subtitle = subtitle
        self.trailing = { EmptyView() }
    }
}

struct OCBanner: View {
    var title: String
    var message: String
    var systemImage: String = "exclamationmark.triangle.fill"
    var tint: Color = .orange

    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: systemImage)
                .foregroundStyle(tint)

            VStack(alignment: .leading, spacing: 2) {
                Text(title)
                    .font(.subheadline)
                    .fontWeight(.semibold)

                Text(message)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .textSelection(.enabled)
                    .fixedSize(horizontal: false, vertical: true)
            }

            Spacer(minLength: 0)
        }
        .padding(12)
        .background(tint.opacity(0.10))
        .clipShape(RoundedRectangle(cornerRadius: OCLayout.cornerRadius))
    }
}

struct PathRow: View {
    var label: String
    var path: String
    var labelWidth: CGFloat = OCLayout.labelColumnWidth
    var showCopy: Bool = true

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Text(label)
                .foregroundStyle(.secondary)
                .frame(width: labelWidth, alignment: .leading)

            Text(path)
                .font(.callout.monospaced())
                .textSelection(.enabled)
                .lineLimit(1)
                .truncationMode(.middle)

            Spacer(minLength: 0)

            if showCopy {
                Button {
                    copyToPasteboard(path)
                } label: {
                    Image(systemName: "doc.on.doc")
                }
                .help("Copy")
                .buttonStyle(.borderless)
            }
        }
    }
}

struct SafetyBadge: View {
    let level: SafetyLevel

    var body: some View {
        Text(level.title)
            .font(.caption2)
            .padding(.horizontal, 6)
            .padding(.vertical, 2)
            .background(level.tint.opacity(0.18))
            .foregroundStyle(level.tint)
            .clipShape(Capsule())
    }
}
